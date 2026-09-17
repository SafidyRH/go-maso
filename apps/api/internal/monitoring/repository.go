package monitoring

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
	}
}

func (r *Repository) Create(
	ctx context.Context,
	check HealthCheck,
) (HealthCheck, error) {
	query := `
		INSERT INTO health_checks (
			application_id,
			status,
			status_code,
			response_time_ms,
			error_message
		)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING
			id::text,
			application_id::text,
			status,
			status_code,
			response_time_ms,
			error_message,
			checked_at
	`

	var result HealthCheck

	err := r.db.QueryRow(
		ctx,
		query,
		check.ApplicationID,
		check.Status,
		check.StatusCode,
		check.ResponseTimeMS,
		check.ErrorMessage,
	).Scan(
		&result.ID,
		&result.ApplicationID,
		&result.Status,
		&result.StatusCode,
		&result.ResponseTimeMS,
		&result.ErrorMessage,
		&result.CheckedAt,
	)

	if err != nil {
		return HealthCheck{}, fmt.Errorf(
			"create health check: %w",
			err,
		)
	}

	return result, nil
}

func (r *Repository) ListByApplication(
	ctx context.Context,
	applicationID string,
	limit int,
) ([]HealthCheck, error) {
	query := `
		SELECT
			id::text,
			application_id::text,
			status,
			status_code,
			response_time_ms,
			error_message,
			checked_at
		FROM health_checks
		WHERE application_id = $1
		ORDER BY checked_at DESC
		LIMIT $2
	`

	rows, err := r.db.Query(
		ctx,
		query,
		applicationID,
		limit,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"list health checks: %w",
			err,
		)
	}

	defer rows.Close()

	checks := make([]HealthCheck, 0)

	for rows.Next() {
		var check HealthCheck

		err := rows.Scan(
			&check.ID,
			&check.ApplicationID,
			&check.Status,
			&check.StatusCode,
			&check.ResponseTimeMS,
			&check.ErrorMessage,
			&check.CheckedAt,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"scan health check: %w",
				err,
			)
		}

		checks = append(checks, check)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate health checks: %w",
			err,
		)
	}

	return checks, nil
}

func (r *Repository) GetSummary(
	ctx context.Context,
	applicationID string,
) (Summary, error) {
	query := `
		WITH stats AS (
			SELECT
				COUNT(*) AS total_checks,

				COUNT(*) FILTER (
					WHERE status = 'UP'
				) AS up_checks,

				COUNT(*) FILTER (
					WHERE status = 'DOWN'
				) AS down_checks,

				COALESCE(
					AVG(response_time_ms) FILTER (
						WHERE status = 'UP'
					),
					0
				) AS average_response_time

			FROM health_checks

			WHERE
				application_id = $1
				AND checked_at >= NOW() - INTERVAL '24 hours'
		),

		latest AS (
			SELECT
				status,
				status_code,
				response_time_ms,
				checked_at

			FROM health_checks

			WHERE application_id = $1

			ORDER BY checked_at DESC

			LIMIT 1
		)

		SELECT
			stats.total_checks,
			stats.up_checks,
			stats.down_checks,
			stats.average_response_time,

			latest.status,
			latest.status_code,
			latest.response_time_ms,
			latest.checked_at

		FROM stats

		LEFT JOIN latest ON TRUE
	`

	var (
		summary Summary

		status         *string
		statusCode     *int
		responseTimeMS *int64
		checkedAt      *time.Time
	)

	err := r.db.QueryRow(
		ctx,
		query,
		applicationID,
	).Scan(
		&summary.TotalChecks24H,
		&summary.UpChecks24H,
		&summary.DownChecks24H,
		&summary.AverageResponseTimeMS,

		&status,
		&statusCode,
		&responseTimeMS,
		&checkedAt,
	)

	if err != nil {
		return Summary{}, fmt.Errorf(
			"get monitoring summary: %w",
			err,
		)
	}

	summary.ApplicationID = applicationID

	if status == nil {
		summary.Status = StatusUnknown
	} else {
		summary.Status = Status(*status)
	}

	if statusCode != nil {
		summary.LastStatusCode = *statusCode
	}

	if responseTimeMS != nil {
		summary.LastResponseTimeMS = *responseTimeMS
	}

	summary.LastCheckedAt = checkedAt

	if summary.TotalChecks24H > 0 {
		summary.Uptime24H =
			float64(summary.UpChecks24H) /
				float64(summary.TotalChecks24H) *
				100
	}

	return summary, nil
}
