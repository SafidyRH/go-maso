package environment

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("environment not found")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(
	ctx context.Context,
	applicationID string,
	input CreateEnvironmentInput,
) (Environment, error) {
	enabled := true
	if input.Enabled != nil {
		enabled = *input.Enabled
	}

	const query = `
		INSERT INTO environments (application_id, name, base_url, enabled)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, application_id::text, name, base_url, enabled,
		          archived_at, created_at, updated_at
	`

	return scanEnvironment(r.db.QueryRow(ctx, query, applicationID, input.Name, input.BaseURL, enabled))
}

func (r *Repository) ListByApplication(
	ctx context.Context,
	applicationID string,
	limit, offset int,
) ([]Environment, error) {
	const query = `
		SELECT e.id::text, e.application_id::text, e.name, e.base_url, e.enabled,
		       e.archived_at, e.created_at, e.updated_at
		FROM environments e
		JOIN applications a ON a.id = e.application_id
		WHERE e.application_id = $1
		  AND e.archived_at IS NULL
		  AND a.archived_at IS NULL
		ORDER BY e.created_at
		LIMIT $2 OFFSET $3
	`

	rows, err := r.db.Query(ctx, query, applicationID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	defer rows.Close()

	environments := make([]Environment, 0)
	for rows.Next() {
		environment, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		environments = append(environments, environment)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate environments: %w", err)
	}

	return environments, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (Environment, error) {
	const query = `
		SELECT e.id::text, e.application_id::text, e.name, e.base_url, e.enabled,
		       e.archived_at, e.created_at, e.updated_at
		FROM environments e
		JOIN applications a ON a.id = e.application_id
		WHERE e.id = $1 AND e.archived_at IS NULL AND a.archived_at IS NULL
	`

	return scanEnvironment(r.db.QueryRow(ctx, query, id))
}

func (r *Repository) Update(ctx context.Context, environment Environment) (Environment, error) {
	const query = `
		UPDATE environments
		SET name = $2, base_url = $3, enabled = $4, updated_at = NOW()
		WHERE id = $1 AND archived_at IS NULL
		RETURNING id::text, application_id::text, name, base_url, enabled,
		          archived_at, created_at, updated_at
	`

	return scanEnvironment(r.db.QueryRow(
		ctx,
		query,
		environment.ID,
		environment.Name,
		environment.BaseURL,
		environment.Enabled,
	))
}

func (r *Repository) Archive(ctx context.Context, id string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE environments
		SET archived_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND archived_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("archive environment: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanEnvironment(row scanner) (Environment, error) {
	var environment Environment
	err := row.Scan(
		&environment.ID,
		&environment.ApplicationID,
		&environment.Name,
		&environment.BaseURL,
		&environment.Enabled,
		&environment.ArchivedAt,
		&environment.CreatedAt,
		&environment.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Environment{}, ErrNotFound
	}
	if err != nil {
		return Environment{}, fmt.Errorf("scan environment: %w", err)
	}

	return environment, nil
}
