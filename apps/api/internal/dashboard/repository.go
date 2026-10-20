package dashboard

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) List(ctx context.Context) ([]ApplicationOverview, error) {
	const query = `
		WITH active_monitors AS (
			SELECT m.*, e.application_id
			FROM monitors m
			JOIN environments e ON e.id = m.environment_id
			WHERE m.archived_at IS NULL
			  AND e.archived_at IS NULL
			  AND m.enabled
			  AND e.enabled
		),
		states AS (
			SELECT
				application_id,
				COUNT(*) AS monitor_count,
				COUNT(*) FILTER (WHERE current_status = 'DOWN') AS down_count,
				COUNT(*) FILTER (WHERE current_status = 'DEGRADED') AS degraded_count,
				COUNT(*) FILTER (WHERE current_status = 'UNKNOWN') AS unknown_count
			FROM active_monitors
			GROUP BY application_id
		),
		stats AS (
			SELECT
				am.application_id,
				COUNT(*) AS total_checks,
				COUNT(*) FILTER (WHERE cr.outcome = 'UP') AS up_checks,
				COUNT(*) FILTER (WHERE cr.outcome = 'DOWN') AS down_checks,
				COALESCE(AVG(cr.response_time_ms) FILTER (WHERE cr.outcome = 'UP'), 0)::double precision AS average_response_time
			FROM active_monitors am
			JOIN check_results cr ON cr.monitor_id = am.id
			WHERE cr.checked_at >= NOW() - INTERVAL '24 hours'
			GROUP BY am.application_id
		),
		latest AS (
			SELECT DISTINCT ON (am.application_id)
				am.application_id,
				cr.status_code,
				cr.response_time_ms,
				cr.checked_at
			FROM active_monitors am
			JOIN check_results cr ON cr.monitor_id = am.id
			ORDER BY am.application_id, cr.checked_at DESC
		)
		SELECT
			a.id::text,
			a.name,
			a.owner,
			CASE
				WHEN COALESCE(states.monitor_count, 0) = 0 THEN 'UNKNOWN'
				WHEN states.down_count > 0 THEN 'DOWN'
				WHEN states.degraded_count > 0 THEN 'DEGRADED'
				WHEN states.unknown_count > 0 THEN 'UNKNOWN'
				ELSE 'UP'
			END,
			COALESCE(states.monitor_count, 0),
			COALESCE(latest.status_code, 0),
			COALESCE(latest.response_time_ms, 0),
			latest.checked_at,
			CASE
				WHEN COALESCE(stats.total_checks, 0) = 0 THEN 0
				ELSE stats.up_checks::double precision / stats.total_checks::double precision * 100
			END,
			COALESCE(stats.average_response_time, 0),
			COALESCE(stats.total_checks, 0),
			COALESCE(stats.up_checks, 0),
			COALESCE(stats.down_checks, 0)
		FROM applications a
		LEFT JOIN states ON states.application_id = a.id
		LEFT JOIN stats ON stats.application_id = a.id
		LEFT JOIN latest ON latest.application_id = a.id
		WHERE a.archived_at IS NULL
		ORDER BY a.created_at DESC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list dashboard applications: %w", err)
	}
	defer rows.Close()

	applications := make([]ApplicationOverview, 0)
	for rows.Next() {
		var app ApplicationOverview
		if err := rows.Scan(
			&app.ID,
			&app.Name,
			&app.Owner,
			&app.Status,
			&app.Monitors,
			&app.LastStatusCode,
			&app.LastResponseTimeMS,
			&app.LastCheckedAt,
			&app.Uptime24H,
			&app.AverageResponseTimeMS,
			&app.TotalChecks24H,
			&app.UpChecks24H,
			&app.DownChecks24H,
		); err != nil {
			return nil, fmt.Errorf("scan dashboard application: %w", err)
		}
		applications = append(applications, app)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dashboard applications: %w", err)
	}

	return applications, nil
}
