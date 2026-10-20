package migrations

import (
	"context"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLoadSortsAndPairsMigrations(t *testing.T) {
	t.Parallel()

	fileSystem := fstest.MapFS{
		"000002_second.down.sql": {Data: []byte("DOWN 2")},
		"000001_first.up.sql":    {Data: []byte("UP 1")},
		"000002_second.up.sql":   {Data: []byte("UP 2")},
		"000001_first.down.sql":  {Data: []byte("DOWN 1")},
	}

	migrations, err := load(fileSystem)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}

	if len(migrations) != 2 {
		t.Fatalf("len(migrations) = %d, want 2", len(migrations))
	}

	if migrations[0].Version != 1 || migrations[1].Version != 2 {
		t.Fatalf("migration order = [%d, %d], want [1, 2]", migrations[0].Version, migrations[1].Version)
	}
}

func TestUpAgainstPostgres(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create database pool: %v", err)
	}
	defer pool.Close()

	if err := Up(ctx, pool); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	for _, table := range []string{"applications", "environments", "monitors", "check_results", "schema_migrations"} {
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass('public.' || $1) IS NOT NULL", table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %s does not exist after migrations", table)
		}
	}

	var versions int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&versions); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if versions != 3 {
		t.Fatalf("applied migration count = %d, want 3", versions)
	}
}

func TestLoadRejectsMissingDirection(t *testing.T) {
	t.Parallel()

	_, err := load(fstest.MapFS{
		"000001_first.up.sql": {Data: []byte("UP 1")},
	})
	if err == nil {
		t.Fatal("load migrations succeeded, want missing down migration error")
	}
}
