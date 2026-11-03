package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUserNotFound    = errors.New("user not found")
	ErrSessionNotFound = errors.New("session not found")
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) BootstrapAdmin(ctx context.Context, email, password string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin admin bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('go-maso-auth-bootstrap'))"); err != nil {
		return fmt.Errorf("lock admin bootstrap: %w", err)
	}
	var count int
	if err := tx.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return tx.Commit(ctx)
	}
	if email == "" || password == "" {
		return errors.New("ADMIN_EMAIL and ADMIN_PASSWORD are required to create the first administrator")
	}
	if len(email) > 254 || !strings.Contains(email, "@") {
		return errors.New("ADMIN_EMAIL must be a valid email address")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash initial administrator password: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (email, password_hash, role)
		VALUES ($1, $2, 'ADMIN')
	`, email, hash); err != nil {
		return fmt.Errorf("create initial administrator: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit admin bootstrap: %w", err)
	}
	return nil
}

func (r *Repository) FindUserByEmail(ctx context.Context, email string) (User, string, error) {
	var user User
	var passwordHash string
	err := r.db.QueryRow(ctx, `
		SELECT id::text, email, password_hash, role, disabled, last_login_at, created_at, updated_at
		FROM users WHERE LOWER(email) = LOWER($1)
	`, strings.TrimSpace(email)).Scan(
		&user.ID,
		&user.Email,
		&passwordHash,
		&user.Role,
		&user.Disabled,
		&user.LastLoginAt,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, "", ErrUserNotFound
	}
	if err != nil {
		return User{}, "", fmt.Errorf("find user by email: %w", err)
	}
	return user, passwordHash, nil
}

func (r *Repository) CreateSession(
	ctx context.Context,
	user User,
	tokenHash, csrfHash []byte,
	remoteAddress, userAgent string,
	expiresAt time.Time,
) (Session, error) {
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	var session Session
	session.User = user
	err := r.db.QueryRow(ctx, `
		INSERT INTO user_sessions (
			user_id, token_hash, csrf_token_hash, remote_address, user_agent, expires_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id::text, csrf_token_hash, last_seen_at, expires_at, created_at
	`, user.ID, tokenHash, csrfHash, remoteAddress, userAgent, expiresAt).Scan(
		&session.ID,
		&session.CSRFTokenHash,
		&session.LastSeenAt,
		&session.ExpiresAt,
		&session.CreatedAt,
	)
	if err != nil {
		return Session{}, fmt.Errorf("create user session: %w", err)
	}
	if _, err := r.db.Exec(ctx, "UPDATE users SET last_login_at = NOW(), updated_at = NOW() WHERE id = $1", user.ID); err != nil {
		return Session{}, fmt.Errorf("update last login: %w", err)
	}
	return session, nil
}

func (r *Repository) FindSession(ctx context.Context, rawToken string, idleTimeout time.Duration) (Session, error) {
	tokenHash := sha256.Sum256([]byte(rawToken))
	var session Session
	err := r.db.QueryRow(ctx, `
		SELECT s.id::text, s.csrf_token_hash, s.last_seen_at, s.expires_at, s.created_at,
		       u.id::text, u.email, u.role, u.disabled, u.last_login_at, u.created_at, u.updated_at
		FROM user_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.expires_at > NOW()
		  AND s.last_seen_at > NOW() - ($2 * INTERVAL '1 second')
		  AND NOT u.disabled
	`, tokenHash[:], int64(idleTimeout.Seconds())).Scan(
		&session.ID,
		&session.CSRFTokenHash,
		&session.LastSeenAt,
		&session.ExpiresAt,
		&session.CreatedAt,
		&session.User.ID,
		&session.User.Email,
		&session.User.Role,
		&session.User.Disabled,
		&session.User.LastLoginAt,
		&session.User.CreatedAt,
		&session.User.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("find user session: %w", err)
	}
	if time.Since(session.LastSeenAt) >= time.Minute {
		_, _ = r.db.Exec(ctx, "UPDATE user_sessions SET last_seen_at = NOW() WHERE id = $1", session.ID)
	}
	return session, nil
}

func (r *Repository) DeleteSession(ctx context.Context, rawToken string) error {
	tokenHash := sha256.Sum256([]byte(rawToken))
	if _, err := r.db.Exec(ctx, "DELETE FROM user_sessions WHERE token_hash = $1", tokenHash[:]); err != nil {
		return fmt.Errorf("delete user session: %w", err)
	}
	return nil
}

func (r *Repository) DeleteExpiredSessions(ctx context.Context, idleTimeout time.Duration) error {
	if _, err := r.db.Exec(ctx, `
		DELETE FROM user_sessions
		WHERE expires_at <= NOW() OR last_seen_at <= NOW() - ($1 * INTERVAL '1 second')
	`, int64(idleTimeout.Seconds())); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}
