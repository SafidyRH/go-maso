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
	"github.com/safidy/go-maso/apps/api/internal/database"
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

	monitoringRepository := monitoring.NewRepository(db)

	checker := monitoring.NewChecker(
		5 * time.Second,
	)

	monitoringService := monitoring.NewService(
		applicationRepository,
		monitoringRepository,
		checker,
	)

	scheduler := monitoring.NewScheduler(
		applicationRepository,
		monitoringService,
	)

	// Application HTTP
	applicationHandler := application.NewHandler(
		applicationRepository,
		scheduler,
	)

	applicationHandler.RegisterRoutes(mux)

	// Monitoring HTTP
	monitoringHandler := monitoring.NewHandler(
		monitoringService,
	)

	monitoringHandler.RegisterRoutes(mux)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := db.Ping(ctx); err != nil {
			http.Error(
				w,
				`{"status":"error","database":"down"}`,
				http.StatusServiceUnavailable,
			)
			return
		}

		response := HealthResponse{
			Status:    "ok",
			Service:   "go-maso-api",
			Database:  "up",
			Timestamp: time.Now(),
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(response); err != nil {
			slog.Error("failed to encode response", "error", err)
		}
	})

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	slog.Info("Go-Maso API started", "port", 8080)

	serverErrors := make(chan error, 1)

	go func() {
		slog.Info(
			"Go-Maso API started",
			"port", 8080,
		)

		serverErrors <- server.ListenAndServe()
	}()

	if err := scheduler.Start(ctx); err != nil {
		slog.Error(
			"failed to start monitoring scheduler",
			"error", err,
		)

		os.Exit(1)
	}

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
		10*time.Second,
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
