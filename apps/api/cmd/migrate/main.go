package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/safidy/go-maso/apps/api/internal/config"
	"github.com/safidy/go-maso/apps/api/internal/database"
	"github.com/safidy/go-maso/apps/api/migrations"
)

func main() {
	if len(os.Args) < 2 {
		fail("usage: go run ./cmd/migrate <up|down|status> [steps]")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		fail("load config: %v", err)
	}

	pool, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		fail("connect to database: %v", err)
	}
	defer pool.Close()

	switch os.Args[1] {
	case "up":
		if err := migrations.Up(ctx, pool); err != nil {
			fail("apply migrations: %v", err)
		}
		slog.Info("database migrations applied")

	case "down":
		steps := 1
		if len(os.Args) >= 3 {
			steps, err = strconv.Atoi(os.Args[2])
			if err != nil || steps < 1 {
				fail("migration steps must be a positive integer")
			}
		}

		if err := migrations.Down(ctx, pool, steps); err != nil {
			fail("rollback migrations: %v", err)
		}
		slog.Info("database migrations rolled back", "steps", steps)

	case "status":
		applied, err := migrations.Status(ctx, pool)
		if err != nil {
			fail("read migration status: %v", err)
		}

		if len(applied) == 0 {
			fmt.Println("No migrations applied.")
			return
		}

		for _, migration := range applied {
			fmt.Printf("%06d  %-40s  %s\n", migration.Version, migration.Name, migration.AppliedAt.UTC().Format("2006-01-02T15:04:05Z"))
		}

	default:
		fail("unknown migration command %q", os.Args[1])
	}
}

func fail(message string, args ...any) {
	slog.Error(fmt.Sprintf(message, args...))
	os.Exit(1)
}
