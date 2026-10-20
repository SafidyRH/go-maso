package monitor

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("monitor not found")

const selectColumns = `
	m.id::text,
	m.environment_id::text,
	m.name,
	m.url,
	m.method,
	m.check_interval_seconds,
	m.timeout_ms,
	m.expected_status_min,
	m.expected_status_max,
	m.failure_threshold,
	m.enabled,
	(m.enabled AND e.enabled AND e.archived_at IS NULL AND a.archived_at IS NULL) AS effective_enabled,
	m.current_status,
	m.consecutive_failures,
	m.last_checked_at,
	m.archived_at,
	m.created_at,
	m.updated_at
`

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(
	ctx context.Context,
	environmentID string,
	input CreateMonitorInput,
) (Monitor, error) {
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	query := `
		WITH inserted AS (
			INSERT INTO monitors (
				environment_id, name, url, method, check_interval_seconds, timeout_ms,
				expected_status_min, expected_status_max, failure_threshold, enabled
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			RETURNING *
		)
		SELECT ` + stringsForInserted(selectColumns) + `
		FROM inserted m
		JOIN environments e ON e.id = m.environment_id
		JOIN applications a ON a.id = e.application_id
	`

	return scanMonitor(r.db.QueryRow(
		ctx,
		query,
		environmentID,
		input.Name,
		input.URL,
		input.Method,
		input.CheckIntervalSeconds,
		input.TimeoutMS,
		input.ExpectedStatusMin,
		input.ExpectedStatusMax,
		input.FailureThreshold,
		enabled,
	))
}

func (r *Repository) GetByID(ctx context.Context, id string) (Monitor, error) {
	query := `
		SELECT ` + selectColumns + `
		FROM monitors m
		JOIN environments e ON e.id = m.environment_id
		JOIN applications a ON a.id = e.application_id
		WHERE m.id = $1
		  AND m.archived_at IS NULL
		  AND e.archived_at IS NULL
		  AND a.archived_at IS NULL
	`

	return scanMonitor(r.db.QueryRow(ctx, query, id))
}

func (r *Repository) ListByEnvironment(
	ctx context.Context,
	environmentID string,
	limit, offset int,
) ([]Monitor, error) {
	query := `
		SELECT ` + selectColumns + `
		FROM monitors m
		JOIN environments e ON e.id = m.environment_id
		JOIN applications a ON a.id = e.application_id
		WHERE m.environment_id = $1 AND m.archived_at IS NULL
		ORDER BY m.created_at
		LIMIT $2 OFFSET $3
	`

	return r.list(ctx, query, environmentID, limit, offset)
}

func (r *Repository) ListEnabled(ctx context.Context) ([]Monitor, error) {
	query := `
		SELECT ` + selectColumns + `
		FROM monitors m
		JOIN environments e ON e.id = m.environment_id
		JOIN applications a ON a.id = e.application_id
		WHERE m.archived_at IS NULL
		  AND m.enabled
		  AND e.enabled
		  AND e.archived_at IS NULL
		  AND a.archived_at IS NULL
		ORDER BY m.created_at
	`

	return r.list(ctx, query)
}

func (r *Repository) ListForSchedulingByEnvironment(
	ctx context.Context,
	environmentID string,
) ([]Monitor, error) {
	query := `
		SELECT ` + selectColumns + `
		FROM monitors m
		JOIN environments e ON e.id = m.environment_id
		JOIN applications a ON a.id = e.application_id
		WHERE m.environment_id = $1 AND m.archived_at IS NULL
	`

	return r.list(ctx, query, environmentID)
}

func (r *Repository) ListForSchedulingByApplication(
	ctx context.Context,
	applicationID string,
) ([]Monitor, error) {
	query := `
		SELECT ` + selectColumns + `
		FROM monitors m
		JOIN environments e ON e.id = m.environment_id
		JOIN applications a ON a.id = e.application_id
		WHERE e.application_id = $1 AND m.archived_at IS NULL
	`

	return r.list(ctx, query, applicationID)
}

func (r *Repository) Update(ctx context.Context, monitor Monitor) (Monitor, error) {
	query := `
		WITH updated AS (
			UPDATE monitors
			SET name = $2,
			    url = $3,
			    method = $4,
			    check_interval_seconds = $5,
			    timeout_ms = $6,
			    expected_status_min = $7,
			    expected_status_max = $8,
			    failure_threshold = $9,
			    enabled = $10,
			    updated_at = NOW()
			WHERE id = $1 AND archived_at IS NULL
			RETURNING *
		)
		SELECT ` + stringsForInserted(selectColumns) + `
		FROM updated m
		JOIN environments e ON e.id = m.environment_id
		JOIN applications a ON a.id = e.application_id
	`

	return scanMonitor(r.db.QueryRow(
		ctx,
		query,
		monitor.ID,
		monitor.Name,
		monitor.URL,
		monitor.Method,
		monitor.CheckIntervalSeconds,
		monitor.TimeoutMS,
		monitor.ExpectedStatusMin,
		monitor.ExpectedStatusMax,
		monitor.FailureThreshold,
		monitor.Enabled,
	))
}

func (r *Repository) Archive(ctx context.Context, id string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE monitors
		SET archived_at = NOW(), enabled = FALSE, updated_at = NOW()
		WHERE id = $1 AND archived_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("archive monitor: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *Repository) list(ctx context.Context, query string, args ...any) ([]Monitor, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list monitors: %w", err)
	}
	defer rows.Close()

	monitors := make([]Monitor, 0)
	for rows.Next() {
		monitor, err := scanMonitor(rows)
		if err != nil {
			return nil, err
		}
		monitors = append(monitors, monitor)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate monitors: %w", err)
	}

	return monitors, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanMonitor(row scanner) (Monitor, error) {
	var monitor Monitor
	err := row.Scan(
		&monitor.ID,
		&monitor.EnvironmentID,
		&monitor.Name,
		&monitor.URL,
		&monitor.Method,
		&monitor.CheckIntervalSeconds,
		&monitor.TimeoutMS,
		&monitor.ExpectedStatusMin,
		&monitor.ExpectedStatusMax,
		&monitor.FailureThreshold,
		&monitor.Enabled,
		&monitor.EffectiveEnabled,
		&monitor.CurrentStatus,
		&monitor.ConsecutiveFailures,
		&monitor.LastCheckedAt,
		&monitor.ArchivedAt,
		&monitor.CreatedAt,
		&monitor.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Monitor{}, ErrNotFound
	}
	if err != nil {
		return Monitor{}, fmt.Errorf("scan monitor: %w", err)
	}

	return monitor, nil
}

func stringsForInserted(columns string) string {
	// CTE rows use the same alias as the table. This helper keeps the shared
	// column list readable while removing no SQL semantics.
	return columns
}
