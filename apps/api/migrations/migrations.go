package migrations

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const advisoryLockID int64 = 4766556276627366

//go:embed *.sql
var files embed.FS

type Migration struct {
	Version int64
	Name    string
	UpSQL   string
	DownSQL string
}

type AppliedMigration struct {
	Version   int64
	Name      string
	AppliedAt time.Time
}

func Up(ctx context.Context, pool *pgxpool.Pool) error {
	migrations, err := load(files)
	if err != nil {
		return err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockID); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", advisoryLockID)
	}()

	if err := ensureTable(ctx, conn); err != nil {
		return err
	}

	applied, err := appliedVersions(ctx, conn)
	if err != nil {
		return err
	}

	for _, migration := range migrations {
		if applied[migration.Version] {
			continue
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", migration.Version, err)
		}

		if _, err := tx.Exec(ctx, migration.UpSQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %d (%s): %w", migration.Version, migration.Name, err)
		}

		if _, err := tx.Exec(
			ctx,
			"INSERT INTO schema_migrations (version, name) VALUES ($1, $2)",
			migration.Version,
			migration.Name,
		); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %d: %w", migration.Version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d: %w", migration.Version, err)
		}
	}

	return nil
}

func Down(ctx context.Context, pool *pgxpool.Pool, steps int) error {
	if steps < 1 {
		return errors.New("migration steps must be greater than zero")
	}

	migrations, err := load(files)
	if err != nil {
		return err
	}

	byVersion := make(map[int64]Migration, len(migrations))
	for _, migration := range migrations {
		byVersion[migration.Version] = migration
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockID); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", advisoryLockID)
	}()

	if err := ensureTable(ctx, conn); err != nil {
		return err
	}

	rows, err := conn.Query(
		ctx,
		"SELECT version FROM schema_migrations ORDER BY version DESC LIMIT $1",
		steps,
	)
	if err != nil {
		return fmt.Errorf("list migrations to rollback: %w", err)
	}

	versions := make([]int64, 0, steps)
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			rows.Close()
			return fmt.Errorf("scan migration version: %w", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate migration versions: %w", err)
	}
	rows.Close()

	for _, version := range versions {
		migration, ok := byVersion[version]
		if !ok {
			return fmt.Errorf("migration %d is applied but its files are missing", version)
		}

		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin rollback %d: %w", version, err)
		}

		if _, err := tx.Exec(ctx, migration.DownSQL); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("rollback migration %d (%s): %w", version, migration.Name, err)
		}

		if _, err := tx.Exec(ctx, "DELETE FROM schema_migrations WHERE version = $1", version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("remove migration record %d: %w", version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit rollback %d: %w", version, err)
		}
	}

	return nil
}

func Status(ctx context.Context, pool *pgxpool.Pool) ([]AppliedMigration, error) {
	if err := ensureTable(ctx, pool); err != nil {
		return nil, err
	}

	rows, err := pool.Query(
		ctx,
		"SELECT version, name, applied_at FROM schema_migrations ORDER BY version",
	)
	if err != nil {
		return nil, fmt.Errorf("list applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make([]AppliedMigration, 0)
	for rows.Next() {
		var migration AppliedMigration
		if err := rows.Scan(&migration.Version, &migration.Name, &migration.AppliedAt); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied = append(applied, migration)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}

	return applied, nil
}

type database interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

func ensureTable(ctx context.Context, db database) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	return nil
}

func appliedVersions(ctx context.Context, db database) (map[int64]bool, error) {
	rows, err := db.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("list applied migration versions: %w", err)
	}
	defer rows.Close()

	applied := make(map[int64]bool)
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan applied migration version: %w", err)
		}
		applied[version] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migration versions: %w", err)
	}

	return applied, nil
}

func load(fileSystem fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fileSystem, ".")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}

	byVersion := make(map[int64]*Migration)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}

		version, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid migration version in %q: %w", entry.Name(), err)
		}

		var direction string
		var name string
		switch {
		case strings.HasSuffix(parts[1], ".up.sql"):
			direction = "up"
			name = strings.TrimSuffix(parts[1], ".up.sql")
		case strings.HasSuffix(parts[1], ".down.sql"):
			direction = "down"
			name = strings.TrimSuffix(parts[1], ".down.sql")
		default:
			return nil, fmt.Errorf("migration %q must end in .up.sql or .down.sql", entry.Name())
		}

		content, err := fs.ReadFile(fileSystem, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}

		migration := byVersion[version]
		if migration == nil {
			migration = &Migration{Version: version, Name: name}
			byVersion[version] = migration
		}

		if migration.Name != name {
			return nil, fmt.Errorf("migration %d has mismatched names", version)
		}

		if direction == "up" {
			migration.UpSQL = string(content)
		} else {
			migration.DownSQL = string(content)
		}
	}

	migrations := make([]Migration, 0, len(byVersion))
	for _, migration := range byVersion {
		if migration.UpSQL == "" || migration.DownSQL == "" {
			return nil, fmt.Errorf("migration %d must have up and down files", migration.Version)
		}
		migrations = append(migrations, *migration)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}
