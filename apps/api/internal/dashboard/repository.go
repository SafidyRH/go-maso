package dashboard

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/safidy/go-maso/apps/api/internal/monitor"
)

var ErrNotFound = errors.New("dashboard application not found")

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

func (r *Repository) GetDetail(ctx context.Context, applicationID string, periodHours int) (ApplicationDetail, error) {
	overview, err := r.getOverview(ctx, applicationID, periodHours)
	if err != nil {
		return ApplicationDetail{}, err
	}

	environments, err := r.listEnvironments(ctx, applicationID)
	if err != nil {
		return ApplicationDetail{}, err
	}

	monitors, err := r.listMonitors(ctx, applicationID, periodHours)
	if err != nil {
		return ApplicationDetail{}, err
	}

	environmentIndex := make(map[string]int, len(environments))
	for index := range environments {
		environments[index].Monitors = make([]MonitorDetail, 0)
		environmentIndex[environments[index].ID] = index
	}
	for _, item := range monitors {
		if index, ok := environmentIndex[item.EnvironmentID]; ok {
			environments[index].Monitors = append(environments[index].Monitors, item)
			environments[index].Status = worstStatus(environments[index].Status, item.Status)
		}
	}

	timeline, err := r.listTimeline(ctx, applicationID, periodHours, bucketSeconds(periodHours))
	if err != nil {
		return ApplicationDetail{}, err
	}
	events, err := r.listRecentEvents(ctx, applicationID)
	if err != nil {
		return ApplicationDetail{}, err
	}

	return ApplicationDetail{
		ApplicationOverview: overview,
		PeriodHours:         periodHours,
		GeneratedAt:         time.Now().UTC(),
		Environments:        environments,
		Timeline:            timeline,
		RecentEvents:        events,
	}, nil
}

func (r *Repository) getOverview(ctx context.Context, applicationID string, periodHours int) (ApplicationOverview, error) {
	const query = `
		WITH active_monitors AS (
			SELECT m.*, e.application_id
			FROM monitors m
			JOIN environments e ON e.id = m.environment_id
			WHERE e.application_id = $1
			  AND m.archived_at IS NULL AND e.archived_at IS NULL
			  AND m.enabled AND e.enabled
		), stats AS (
			SELECT COUNT(*) AS total_checks,
			       COUNT(*) FILTER (WHERE cr.outcome = 'UP') AS up_checks,
			       COUNT(*) FILTER (WHERE cr.outcome = 'DOWN') AS down_checks,
			       COALESCE(AVG(cr.response_time_ms) FILTER (WHERE cr.outcome = 'UP'), 0)::double precision AS average_response_time
			FROM active_monitors am
			JOIN check_results cr ON cr.monitor_id = am.id
			WHERE cr.checked_at >= NOW() - ($2 * INTERVAL '1 hour')
		), latest AS (
			SELECT cr.status_code, cr.response_time_ms, cr.checked_at
			FROM active_monitors am
			JOIN check_results cr ON cr.monitor_id = am.id
			ORDER BY cr.checked_at DESC LIMIT 1
		), states AS (
			SELECT COUNT(*) AS monitor_count,
			       COUNT(*) FILTER (WHERE current_status = 'DOWN') AS down_count,
			       COUNT(*) FILTER (WHERE current_status = 'DEGRADED') AS degraded_count,
			       COUNT(*) FILTER (WHERE current_status = 'UNKNOWN') AS unknown_count
			FROM active_monitors
		)
		SELECT a.id::text, a.name, a.owner,
		       CASE WHEN states.monitor_count = 0 THEN 'UNKNOWN'
		            WHEN states.down_count > 0 THEN 'DOWN'
		            WHEN states.degraded_count > 0 THEN 'DEGRADED'
		            WHEN states.unknown_count > 0 THEN 'UNKNOWN' ELSE 'UP' END,
		       states.monitor_count,
		       COALESCE(latest.status_code, 0), COALESCE(latest.response_time_ms, 0), latest.checked_at,
		       CASE WHEN stats.total_checks = 0 THEN 0 ELSE stats.up_checks::double precision / stats.total_checks * 100 END,
		       stats.average_response_time, stats.total_checks, stats.up_checks, stats.down_checks
		FROM applications a CROSS JOIN states CROSS JOIN stats LEFT JOIN latest ON TRUE
		WHERE a.id = $1 AND a.archived_at IS NULL`

	var overview ApplicationOverview
	err := r.db.QueryRow(ctx, query, applicationID, periodHours).Scan(
		&overview.ID, &overview.Name, &overview.Owner, &overview.Status, &overview.Monitors,
		&overview.LastStatusCode, &overview.LastResponseTimeMS, &overview.LastCheckedAt,
		&overview.Uptime24H, &overview.AverageResponseTimeMS, &overview.TotalChecks24H,
		&overview.UpChecks24H, &overview.DownChecks24H,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ApplicationOverview{}, ErrNotFound
	}
	if err != nil {
		return ApplicationOverview{}, fmt.Errorf("get dashboard application: %w", err)
	}
	return overview, nil
}

func (r *Repository) listEnvironments(ctx context.Context, applicationID string) ([]EnvironmentDetail, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, name, base_url, enabled
		FROM environments
		WHERE application_id = $1 AND archived_at IS NULL
		ORDER BY name`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list dashboard environments: %w", err)
	}
	defer rows.Close()

	items := make([]EnvironmentDetail, 0)
	for rows.Next() {
		item := EnvironmentDetail{Status: monitor.StatusUnknown}
		if err := rows.Scan(&item.ID, &item.Name, &item.BaseURL, &item.Enabled); err != nil {
			return nil, fmt.Errorf("scan dashboard environment: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dashboard environments: %w", err)
	}
	return items, nil
}

func (r *Repository) listMonitors(ctx context.Context, applicationID string, periodHours int) ([]MonitorDetail, error) {
	rows, err := r.db.Query(ctx, `
		SELECT m.id::text, m.environment_id::text, m.name, m.url, m.method, m.enabled,
		       (m.enabled AND e.enabled) AS effective_enabled, m.current_status,
		       COALESCE(latest.status_code, 0), COALESCE(latest.response_time_ms, 0), latest.checked_at,
		       CASE WHEN stats.total_checks = 0 THEN 0 ELSE stats.up_checks::double precision / stats.total_checks * 100 END,
		       stats.average_response_time, stats.total_checks
		FROM monitors m
		JOIN environments e ON e.id = m.environment_id
		LEFT JOIN LATERAL (
			SELECT status_code, response_time_ms, checked_at
			FROM check_results WHERE monitor_id = m.id ORDER BY checked_at DESC LIMIT 1
		) latest ON TRUE
		CROSS JOIN LATERAL (
			SELECT COUNT(*) AS total_checks,
			       COUNT(*) FILTER (WHERE outcome = 'UP') AS up_checks,
			       COALESCE(AVG(response_time_ms) FILTER (WHERE outcome = 'UP'), 0)::double precision AS average_response_time
			FROM check_results
			WHERE monitor_id = m.id AND checked_at >= NOW() - ($2 * INTERVAL '1 hour')
		) stats
		WHERE e.application_id = $1 AND e.archived_at IS NULL AND m.archived_at IS NULL
		ORDER BY e.name, m.name`, applicationID, periodHours)
	if err != nil {
		return nil, fmt.Errorf("list dashboard monitors: %w", err)
	}
	defer rows.Close()

	items := make([]MonitorDetail, 0)
	for rows.Next() {
		var item MonitorDetail
		if err := rows.Scan(
			&item.ID, &item.EnvironmentID, &item.Name, &item.URL, &item.Method, &item.Enabled,
			&item.EffectiveEnabled, &item.Status, &item.LastStatusCode, &item.LastResponseTimeMS,
			&item.LastCheckedAt, &item.Uptime, &item.AverageResponseTimeMS, &item.TotalChecks,
		); err != nil {
			return nil, fmt.Errorf("scan dashboard monitor: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dashboard monitors: %w", err)
	}
	return items, nil
}

func (r *Repository) listTimeline(ctx context.Context, applicationID string, periodHours, bucketSizeSeconds int) ([]MetricPoint, error) {
	rows, err := r.db.Query(ctx, `
		SELECT date_bin($3 * INTERVAL '1 second', cr.checked_at, TIMESTAMPTZ '1970-01-01') AS bucket,
		       CASE WHEN COUNT(*) = 0 THEN 0 ELSE COUNT(*) FILTER (WHERE cr.outcome = 'UP')::double precision / COUNT(*) * 100 END,
		       COALESCE(AVG(cr.response_time_ms) FILTER (WHERE cr.outcome = 'UP'), 0)::double precision,
		       COUNT(*)
		FROM check_results cr
		JOIN monitors m ON m.id = cr.monitor_id
		JOIN environments e ON e.id = m.environment_id
		WHERE e.application_id = $1 AND e.archived_at IS NULL AND m.archived_at IS NULL
		  AND cr.checked_at >= NOW() - ($2 * INTERVAL '1 hour')
		GROUP BY bucket ORDER BY bucket`, applicationID, periodHours, bucketSizeSeconds)
	if err != nil {
		return nil, fmt.Errorf("list dashboard timeline: %w", err)
	}
	defer rows.Close()

	items := make([]MetricPoint, 0)
	for rows.Next() {
		var item MetricPoint
		if err := rows.Scan(&item.Timestamp, &item.Uptime, &item.AverageResponseTimeMS, &item.Checks); err != nil {
			return nil, fmt.Errorf("scan dashboard timeline: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dashboard timeline: %w", err)
	}
	return items, nil
}

func (r *Repository) listRecentEvents(ctx context.Context, applicationID string) ([]RecentEvent, error) {
	rows, err := r.db.Query(ctx, `
		SELECT cr.id::text, m.id::text, m.name, e.name, cr.outcome, cr.computed_status,
		       cr.status_code, cr.response_time_ms, cr.error_message, cr.checked_at
		FROM check_results cr
		JOIN monitors m ON m.id = cr.monitor_id
		JOIN environments e ON e.id = m.environment_id
		WHERE e.application_id = $1 AND e.archived_at IS NULL AND m.archived_at IS NULL
		ORDER BY cr.checked_at DESC LIMIT 30`, applicationID)
	if err != nil {
		return nil, fmt.Errorf("list dashboard events: %w", err)
	}
	defer rows.Close()

	items := make([]RecentEvent, 0)
	for rows.Next() {
		var item RecentEvent
		if err := rows.Scan(
			&item.ID, &item.MonitorID, &item.MonitorName, &item.EnvironmentName,
			&item.Outcome, &item.ComputedStatus, &item.StatusCode, &item.ResponseTimeMS,
			&item.ErrorMessage, &item.CheckedAt,
		); err != nil {
			return nil, fmt.Errorf("scan dashboard event: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dashboard events: %w", err)
	}
	return items, nil
}

func bucketSeconds(periodHours int) int {
	switch {
	case periodHours <= 1:
		return 60
	case periodHours <= 6:
		return 300
	case periodHours <= 24:
		return 900
	case periodHours <= 168:
		return 3600
	default:
		return 14400
	}
}

func worstStatus(current, candidate monitor.Status) monitor.Status {
	priority := map[monitor.Status]int{
		monitor.StatusUnknown:  1,
		monitor.StatusUp:       2,
		monitor.StatusDegraded: 3,
		monitor.StatusDown:     4,
	}
	if priority[candidate] > priority[current] {
		return candidate
	}
	return current
}
