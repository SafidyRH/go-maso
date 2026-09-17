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
	return &Repository{
		db: db,
	}
}

func (r *Repository) List(
	ctx context.Context,
) ([]ApplicationOverview, error) {
	query := `
		WITH stats AS (
			SELECT
				application_id,

				COUNT(*) FILTER (
					WHERE checked_at >= NOW() - INTERVAL '24 hours'
				) AS total_checks,

				COUNT(*) FILTER (
					WHERE
						checked_at >= NOW() - INTERVAL '24 hours'
						AND status = 'UP'
				) AS up_checks,

				COUNT(*) FILTER (
					WHERE
						checked_at >= NOW() - INTERVAL '24 hours'
						AND status = 'DOWN'
				) AS down_checks,

				COALESCE(
					AVG(response_time_ms) FILTER (
						WHERE
							checked_at >= NOW() - INTERVAL '24 hours'
							AND status = 'UP'
					),
					0
				)::double precision AS average_response_time

			FROM health_checks

			GROUP BY application_id
		),

		latest AS (
			SELECT DISTINCT ON (application_id)
				application_id,
				status,
				status_code,
				response_time_ms,
				checked_at

			FROM health_checks

			ORDER BY
				application_id,
				checked_at DESC
		)

		SELECT
			a.id::text,
			a.name,
			a.url,
			a.health_check_url,
			a.enabled,

			COALESCE(latest.status, 'UNKNOWN'),
			COALESCE(latest.status_code, 0),
			COALESCE(latest.response_time_ms, 0),
			latest.checked_at,

			CASE
				WHEN COALESCE(stats.total_checks, 0) = 0
					THEN 0

				ELSE (
					COALESCE(stats.up_checks, 0)::double precision
					/
					stats.total_checks::double precision
				) * 100
			END AS uptime_24h,

			COALESCE(
				stats.average_response_time,
				0
			),

			COALESCE(stats.total_checks, 0),
			COALESCE(stats.up_checks, 0),
			COALESCE(stats.down_checks, 0)

		FROM applications a

		LEFT JOIN stats
			ON stats.application_id = a.id

		LEFT JOIN latest
			ON latest.application_id = a.id

		ORDER BY a.created_at DESC
	`

	rows, err := r.db.Query(
		ctx,
		query,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"list dashboard applications: %w",
			err,
		)
	}

	defer rows.Close()

	applications := make(
		[]ApplicationOverview,
		0,
	)

	for rows.Next() {
		var app ApplicationOverview

		err := rows.Scan(
			&app.ID,
			&app.Name,
			&app.URL,
			&app.HealthCheckURL,
			&app.Enabled,

			&app.Status,
			&app.LastStatusCode,
			&app.LastResponseTimeMS,
			&app.LastCheckedAt,

			&app.Uptime24H,
			&app.AverageResponseTimeMS,

			&app.TotalChecks24H,
			&app.UpChecks24H,
			&app.DownChecks24H,
		)

		if err != nil {
			return nil, fmt.Errorf(
				"scan dashboard application: %w",
				err,
			)
		}

		applications = append(
			applications,
			app,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate dashboard applications: %w",
			err,
		)
	}

	return applications, nil
}
