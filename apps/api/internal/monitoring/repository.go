package monitoring

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
