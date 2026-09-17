package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
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

var ErrNotFound = errors.New("application not found")

func (r *Repository) Create(
	ctx context.Context,
	input CreateApplicationInput,
) (Application, error) {
	query := `
		INSERT INTO applications (
			name,
			url,
			health_check_url,
			check_interval_seconds
		)
		VALUES ($1, $2, $3, $4)
		RETURNING
			id::text,
			name,
			url,
			health_check_url,
			check_interval_seconds,
			enabled,
			created_at,
			updated_at
	`

	var app Application

	err := r.db.QueryRow(
		ctx,
		query,
		input.Name,
		input.URL,
		input.HealthCheckURL,
		input.CheckIntervalSeconds,
	).Scan(
		&app.ID,
		&app.Name,
		&app.URL,
		&app.HealthCheckURL,
		&app.CheckIntervalSeconds,
		&app.Enabled,
		&app.CreatedAt,
		&app.UpdatedAt,
	)

	if err != nil {
		return Application{}, fmt.Errorf("create application: %w", err)
	}

	return app, nil
}

func (r *Repository) List(ctx context.Context) ([]Application, error) {
	query := `
		SELECT
			id::text,
			name,
			url,
			health_check_url,
			check_interval_seconds,
			enabled,
			created_at,
			updated_at
		FROM applications
		ORDER BY created_at DESC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}

	defer rows.Close()

	applications := make([]Application, 0)

	for rows.Next() {
		var app Application

		err := rows.Scan(
			&app.ID,
			&app.Name,
			&app.URL,
			&app.HealthCheckURL,
			&app.CheckIntervalSeconds,
			&app.Enabled,
			&app.CreatedAt,
			&app.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("scan application: %w", err)
		}

		applications = append(applications, app)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applications: %w", err)
	}

	return applications, nil
}

func (r *Repository) GetByID(
	ctx context.Context,
	id string,
) (Application, error) {
	query := `
		SELECT
			id::text,
			name,
			url,
			health_check_url,
			check_interval_seconds,
			enabled,
			created_at,
			updated_at
		FROM applications
		WHERE id = $1
	`

	var app Application

	err := r.db.QueryRow(
		ctx,
		query,
		id,
	).Scan(
		&app.ID,
		&app.Name,
		&app.URL,
		&app.HealthCheckURL,
		&app.CheckIntervalSeconds,
		&app.Enabled,
		&app.CreatedAt,
		&app.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNotFound
	}

	if err != nil {
		return Application{}, fmt.Errorf("get application: %w", err)
	}

	return app, nil
}

func (r *Repository) ListEnabled(
	ctx context.Context,
) ([]Application, error) {
	query := `
		SELECT
			id::text,
			name,
			url,
			health_check_url,
			check_interval_seconds,
			enabled,
			created_at,
			updated_at
		FROM applications
		WHERE enabled = TRUE
		ORDER BY created_at ASC
	`

	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list enabled applications: %w", err)
	}

	defer rows.Close()

	applications := make([]Application, 0)

	for rows.Next() {
		var app Application

		if err := rows.Scan(
			&app.ID,
			&app.Name,
			&app.URL,
			&app.HealthCheckURL,
			&app.CheckIntervalSeconds,
			&app.Enabled,
			&app.CreatedAt,
			&app.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan application: %w", err)
		}

		applications = append(applications, app)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applications: %w", err)
	}

	return applications, nil
}

func (r *Repository) Update(
	ctx context.Context,
	app Application,
) (Application, error) {
	query := `
		UPDATE applications
		SET
			name = $2,
			url = $3,
			health_check_url = $4,
			check_interval_seconds = $5,
			enabled = $6,
			updated_at = NOW()
		WHERE id = $1
		RETURNING
			id::text,
			name,
			url,
			health_check_url,
			check_interval_seconds,
			enabled,
			created_at,
			updated_at
	`

	var updated Application

	err := r.db.QueryRow(
		ctx,
		query,
		app.ID,
		app.Name,
		app.URL,
		app.HealthCheckURL,
		app.CheckIntervalSeconds,
		app.Enabled,
	).Scan(
		&updated.ID,
		&updated.Name,
		&updated.URL,
		&updated.HealthCheckURL,
		&updated.CheckIntervalSeconds,
		&updated.Enabled,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNotFound
	}

	if err != nil {
		return Application{}, fmt.Errorf(
			"update application: %w",
			err,
		)
	}

	return updated, nil
}

func (r *Repository) Delete(
	ctx context.Context,
	id string,
) error {
	result, err := r.db.Exec(
		ctx,
		`
			DELETE FROM applications
			WHERE id = $1
		`,
		id,
	)

	if err != nil {
		return fmt.Errorf(
			"delete application: %w",
			err,
		)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}
