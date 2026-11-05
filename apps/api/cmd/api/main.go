package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/safidy/go-maso/apps/api/internal/application"
	"github.com/safidy/go-maso/apps/api/internal/auth"
	"github.com/safidy/go-maso/apps/api/internal/config"
	"github.com/safidy/go-maso/apps/api/internal/dashboard"
	"github.com/safidy/go-maso/apps/api/internal/database"
	"github.com/safidy/go-maso/apps/api/internal/environment"
	"github.com/safidy/go-maso/apps/api/internal/incident"
	"github.com/safidy/go-maso/apps/api/internal/monitor"
	"github.com/safidy/go-maso/apps/api/internal/monitoring"
	"github.com/safidy/go-maso/apps/api/internal/securestore"
	"github.com/safidy/go-maso/apps/api/internal/webui"
)

type HealthResponse struct {
	Status    string    `json:"status"`
	Service   string    `json:"service"`
	Database  string    `json:"database"`
	Timestamp time.Time `json:"timestamp"`
}

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)

	defer stop()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	db, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	defer db.Close()

	slog.Info("connected to PostgreSQL")

	mux := http.NewServeMux()

	applicationRepository := application.NewRepository(db)
	environmentRepository := environment.NewRepository(db)
	monitorRepository := monitor.NewRepository(db)

	monitoringRepository := monitoring.NewRepository(db)
	secretVault, err := securestore.NewVault(cfg.SecretsEncryptionKey)
	if err != nil {
		slog.Error("failed to initialize secret vault", "error", err)
		os.Exit(1)
	}
	incidentRepository := incident.NewRepository(db, secretVault)
	if err := incidentRepository.MigrateLegacySecrets(ctx); err != nil {
		slog.Error("failed to encrypt legacy webhook destinations", "error", err)
		os.Exit(1)
	}
	incidentEngine := incident.NewEngine(incidentRepository)

	dashboardRepository := dashboard.NewRepository(db)
	authRepository := auth.NewRepository(db)
	if err := authRepository.BootstrapAdmin(ctx, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		slog.Error("failed to bootstrap administrator", "error", err)
		os.Exit(1)
	}
	if err := authRepository.DeleteExpiredSessions(ctx, cfg.SessionIdle); err != nil {
		slog.Warn("failed to delete expired sessions", "error", err)
	}
	authService, err := auth.NewService(authRepository, cfg.SessionAbsolute)
	if err != nil {
		slog.Error("failed to initialize authentication", "error", err)
		os.Exit(1)
	}
	authMiddleware := auth.NewMiddleware(authRepository, cfg.SessionIdle, cfg.SessionSecure)
	authHandler := auth.NewHandler(authService, authRepository, authMiddleware, cfg.TrustProxy)

	networkPolicy := monitoring.NewNetworkPolicy()
	checker := monitoring.NewChecker(networkPolicy)
	notificationSender := incident.NewWebhookSender(
		monitoring.NewSafeHTTPClient(networkPolicy, 10*time.Second),
	)
	notificationDispatcher := incident.NewDispatcher(incidentRepository, notificationSender)

	dashboardHandler := dashboard.NewHandler(
		dashboardRepository,
	)

	monitoringService := monitoring.NewService(
		monitorRepository,
		monitoringRepository,
		checker,
		incidentEngine,
	)

	scheduler := monitoring.NewScheduler(
		monitorRepository,
		monitoringService,
		cfg.MonitorWorkers,
		time.Duration(cfg.RetentionDays)*24*time.Hour,
	)

	applicationHandler := application.NewHandler(
		applicationRepository,
		scheduler,
	)
	environmentHandler := environment.NewHandler(
		environmentRepository,
		applicationRepository,
		scheduler,
	)
	monitorHandler := monitor.NewHandler(
		monitorRepository,
		environmentRepository,
		scheduler,
		networkPolicy,
	)
	incidentHandler := incident.NewHandler(
		incidentRepository,
		monitorRepository,
		networkPolicy,
	)

	applicationHandler.RegisterRoutes(mux)
	environmentHandler.RegisterRoutes(mux)
	monitorHandler.RegisterRoutes(mux)
	incidentHandler.RegisterRoutes(mux)
	authHandler.RegisterRoutes(mux)

	monitoringHandler := monitoring.NewHandler(
		monitoringService,
	)

	monitoringHandler.RegisterRoutes(mux)
	dashboardHandler.RegisterRoutes(mux)

	livenessHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		response := HealthResponse{
			Status:    "ok",
			Service:   "go-maso-api",
			Database:  "unchecked",
			Timestamp: time.Now().UTC(),
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			slog.Error("failed to encode liveness response", "error", err)
		}
	}

	readinessHandler := func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		w.Header().Set("Content-Type", "application/json")

		if err := db.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(HealthResponse{
				Status:    "error",
				Service:   "go-maso-api",
				Database:  "down",
				Timestamp: time.Now().UTC(),
			})
			return
		}

		response := HealthResponse{
			Status:    "ok",
			Service:   "go-maso-api",
			Database:  "up",
			Timestamp: time.Now().UTC(),
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			slog.Error("failed to encode response", "error", err)
		}
	}

	mux.HandleFunc("GET /healthz", livenessHandler)
	mux.HandleFunc("GET /readyz", readinessHandler)
	// Kept as a compatibility alias for the endpoint exposed by the prototype.
	mux.HandleFunc("GET /health", readinessHandler)

	webui.NewHandler().RegisterRoutes(mux)

	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           authMiddleware.Handler(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      70 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	serverErrors := make(chan error, 1)

	if err := scheduler.Start(ctx); err != nil {
		slog.Error(
			"failed to start monitoring scheduler",
			"error", err,
		)

		os.Exit(1)
	}
	notificationDispatcher.Start(ctx)

	go func() {
		slog.Info(
			"Go-Maso API started",
			"address", cfg.HTTPAddress,
		)

		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")

	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			slog.Error(
				"HTTP server failed",
				"error", err,
			)
		}

		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		cfg.ShutdownTimeout,
	)

	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error(
			"HTTP server shutdown failed",
			"error", err,
		)
	}

	scheduler.Wait()
	notificationDispatcher.Wait()

	slog.Info("Go-Maso stopped")

}
