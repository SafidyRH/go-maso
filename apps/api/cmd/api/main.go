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
	"github.com/safidy/go-maso/apps/api/internal/config"
	"github.com/safidy/go-maso/apps/api/internal/dashboard"
	"github.com/safidy/go-maso/apps/api/internal/database"
	"github.com/safidy/go-maso/apps/api/internal/environment"
	"github.com/safidy/go-maso/apps/api/internal/monitor"
	"github.com/safidy/go-maso/apps/api/internal/monitoring"
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

	dashboardRepository := dashboard.NewRepository(db)

	networkPolicy := monitoring.NewNetworkPolicy()
	checker := monitoring.NewChecker(networkPolicy)

	dashboardHandler := dashboard.NewHandler(
		dashboardRepository,
	)

	monitoringService := monitoring.NewService(
		monitorRepository,
		monitoringRepository,
		checker,
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

	applicationHandler.RegisterRoutes(mux)
	environmentHandler.RegisterRoutes(mux)
	monitorHandler.RegisterRoutes(mux)

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

	server := &http.Server{
		Addr:              cfg.HTTPAddress,
		Handler:           mux,
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

	slog.Info("Go-Maso stopped")

}
