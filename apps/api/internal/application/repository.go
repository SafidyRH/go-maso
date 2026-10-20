package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("application not found")

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, input CreateApplicationInput) (Application, error) {
	const query = `
		INSERT INTO applications (name, owner)
		VALUES ($1, $2)
		RETURNING id::text, name, owner, archived_at, created_at, updated_at
	`

	return scanApplication(r.db.QueryRow(ctx, query, input.Name, input.Owner))
}

func (r *Repository) List(ctx context.Context, limit, offset int) ([]Application, error) {
	const query = `
		SELECT id::text, name, owner, archived_at, created_at, updated_at
		FROM applications
		WHERE archived_at IS NULL
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := r.db.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	defer rows.Close()

	applications := make([]Application, 0)
	for rows.Next() {
		app, err := scanApplication(rows)
		if err != nil {
			return nil, err
		}
		applications = append(applications, app)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applications: %w", err)
	}

	return applications, nil
}

func (r *Repository) GetByID(ctx context.Context, id string) (Application, error) {
	const query = `
		SELECT id::text, name, owner, archived_at, created_at, updated_at
		FROM applications
		WHERE id = $1 AND archived_at IS NULL
	`

	return scanApplication(r.db.QueryRow(ctx, query, id))
}

func (r *Repository) Update(ctx context.Context, app Application) (Application, error) {
	const query = `
		UPDATE applications
		SET name = $2, owner = $3, updated_at = NOW()
		WHERE id = $1 AND archived_at IS NULL
		RETURNING id::text, name, owner, archived_at, created_at, updated_at
	`

	return scanApplication(r.db.QueryRow(ctx, query, app.ID, app.Name, app.Owner))
}

func (r *Repository) Archive(ctx context.Context, id string) error {
	result, err := r.db.Exec(ctx, `
		UPDATE applications
		SET archived_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND archived_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("archive application: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanApplication(row scanner) (Application, error) {
	var app Application
	err := row.Scan(&app.ID, &app.Name, &app.Owner, &app.ArchivedAt, &app.CreatedAt, &app.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Application{}, ErrNotFound
	}
	if err != nil {
		return Application{}, fmt.Errorf("scan application: %w", err)
	}

	return app, nil
}
