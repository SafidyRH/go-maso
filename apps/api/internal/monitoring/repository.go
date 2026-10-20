package monitoring

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/safidy/go-maso/apps/api/internal/monitor"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Record(ctx context.Context, check CheckResult) (CheckResult, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return CheckResult{}, fmt.Errorf("begin check result transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var failureThreshold int
	var consecutiveFailures int
	err = tx.QueryRow(ctx, `
		SELECT failure_threshold, consecutive_failures
		FROM monitors
		WHERE id = $1 AND archived_at IS NULL
		FOR UPDATE
	`, check.MonitorID).Scan(&failureThreshold, &consecutiveFailures)
	if errors.Is(err, pgx.ErrNoRows) {
		return CheckResult{}, monitor.ErrNotFound
	}
	if err != nil {
		return CheckResult{}, fmt.Errorf("lock monitor state: %w", err)
	}

	status, consecutiveFailures := calculateState(check.Outcome, failureThreshold, consecutiveFailures)

	err = tx.QueryRow(ctx, `
		INSERT INTO check_results (
			monitor_id, outcome, computed_status, consecutive_failures,
			status_code, response_time_ms, error_message
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id::text, monitor_id::text, outcome, computed_status,
		          consecutive_failures, status_code, response_time_ms,
		          error_message, checked_at
	`,
		check.MonitorID,
		check.Outcome,
		status,
		consecutiveFailures,
		check.StatusCode,
		check.ResponseTimeMS,
		check.ErrorMessage,
	).Scan(
		&check.ID,
		&check.MonitorID,
		&check.Outcome,
		&check.ComputedStatus,
		&check.ConsecutiveFailures,
		&check.StatusCode,
		&check.ResponseTimeMS,
		&check.ErrorMessage,
		&check.CheckedAt,
	)
	if err != nil {
		return CheckResult{}, fmt.Errorf("insert check result: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE monitors
		SET current_status = $2,
		    consecutive_failures = $3,
		    last_checked_at = $4,
		    updated_at = NOW()
		WHERE id = $1
	`, check.MonitorID, status, consecutiveFailures, check.CheckedAt); err != nil {
		return CheckResult{}, fmt.Errorf("update monitor state: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return CheckResult{}, fmt.Errorf("commit check result: %w", err)
	}

	check.ComputedStatus = status
	check.ConsecutiveFailures = consecutiveFailures
	return check, nil
}

func calculateState(outcome Outcome, failureThreshold, previousFailures int) (monitor.Status, int) {
	if outcome == OutcomeUp {
		return monitor.StatusUp, 0
	}

	failures := previousFailures + 1
	if failures >= failureThreshold {
		return monitor.StatusDown, failures
	}

	return monitor.StatusDegraded, failures
}

func (r *Repository) ListByMonitor(
	ctx context.Context,
	monitorID string,
	limit int,
) ([]CheckResult, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, monitor_id::text, outcome, computed_status,
		       consecutive_failures, status_code, response_time_ms,
		       error_message, checked_at
		FROM check_results
		WHERE monitor_id = $1
		ORDER BY checked_at DESC
		LIMIT $2
	`, monitorID, limit)
	if err != nil {
		return nil, fmt.Errorf("list check results: %w", err)
	}
	defer rows.Close()

	results := make([]CheckResult, 0)
	for rows.Next() {
		var result CheckResult
		if err := rows.Scan(
			&result.ID,
			&result.MonitorID,
			&result.Outcome,
			&result.ComputedStatus,
			&result.ConsecutiveFailures,
			&result.StatusCode,
			&result.ResponseTimeMS,
			&result.ErrorMessage,
			&result.CheckedAt,
		); err != nil {
			return nil, fmt.Errorf("scan check result: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate check results: %w", err)
	}

	return results, nil
}

func (r *Repository) GetSummary(ctx context.Context, monitorID string) (Summary, error) {
	const query = `
		WITH stats AS (
			SELECT
				COUNT(*) AS total_checks,
				COUNT(*) FILTER (WHERE outcome = 'UP') AS up_checks,
				COUNT(*) FILTER (WHERE outcome = 'DOWN') AS down_checks,
				COALESCE(AVG(response_time_ms) FILTER (WHERE outcome = 'UP'), 0)::double precision AS average_response_time
			FROM check_results
			WHERE monitor_id = $1
			  AND checked_at >= NOW() - INTERVAL '24 hours'
		),
		latest AS (
			SELECT status_code, response_time_ms, checked_at
			FROM check_results
			WHERE monitor_id = $1
			ORDER BY checked_at DESC
			LIMIT 1
		)
		SELECT
			m.current_status,
			m.consecutive_failures,
			COALESCE(latest.status_code, 0),
			COALESCE(latest.response_time_ms, 0),
			latest.checked_at,
			stats.total_checks,
			stats.up_checks,
			stats.down_checks,
			stats.average_response_time
		FROM monitors m
		CROSS JOIN stats
		LEFT JOIN latest ON TRUE
		WHERE m.id = $1 AND m.archived_at IS NULL
	`

	var summary Summary
	err := r.db.QueryRow(ctx, query, monitorID).Scan(
		&summary.Status,
		&summary.ConsecutiveFailures,
		&summary.LastStatusCode,
		&summary.LastResponseTimeMS,
		&summary.LastCheckedAt,
		&summary.TotalChecks24H,
		&summary.UpChecks24H,
		&summary.DownChecks24H,
		&summary.AverageResponseTimeMS,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Summary{}, monitor.ErrNotFound
	}
	if err != nil {
		return Summary{}, fmt.Errorf("get monitoring summary: %w", err)
	}

	summary.MonitorID = monitorID
	if summary.TotalChecks24H > 0 {
		summary.Uptime24H = float64(summary.UpChecks24H) / float64(summary.TotalChecks24H) * 100
	}

	return summary, nil
}

func (r *Repository) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := r.db.Exec(ctx, "DELETE FROM check_results WHERE checked_at < $1", cutoff)
	if err != nil {
		return 0, fmt.Errorf("delete expired check results: %w", err)
	}

	return result.RowsAffected(), nil
}
