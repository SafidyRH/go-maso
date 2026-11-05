package incident

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type PendingDelivery struct {
	ID             string
	IncidentID     string
	EventType      string
	WebhookURL     string
	Payload        []byte
	Attempts       int
	MaxAttempts    int
	IdempotencyKey string
}

var ErrNoPendingDelivery = errors.New("no pending notification delivery")

func (r *Repository) RecoverInterruptedDeliveries(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `
		UPDATE notification_deliveries
		SET status = 'FAILED',
		    last_error = 'delivery interrupted before completion',
		    next_attempt_at = NOW(),
		    updated_at = NOW()
		WHERE status = 'SENDING'
	`)
	if err != nil {
		return fmt.Errorf("recover interrupted deliveries: %w", err)
	}
	return nil
}

func (r *Repository) ClaimPendingDelivery(ctx context.Context) (PendingDelivery, error) {
	var value PendingDelivery
	var encryptedURL []byte
	err := r.db.QueryRow(ctx, `
		WITH candidate AS (
			SELECT id
			FROM notification_deliveries
			WHERE status IN ('PENDING', 'FAILED')
			  AND attempts < max_attempts
			  AND next_attempt_at <= NOW()
			ORDER BY next_attempt_at, created_at
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		UPDATE notification_deliveries d
		SET status = 'SENDING', attempts = attempts + 1, updated_at = NOW()
		FROM candidate
		WHERE d.id = candidate.id
		RETURNING d.id::text, d.incident_id::text, d.event_type, d.webhook_url,
		          d.webhook_url_encrypted, d.payload, d.attempts, d.max_attempts, d.idempotency_key
	`).Scan(
		&value.ID,
		&value.IncidentID,
		&value.EventType,
		&value.WebhookURL,
		&encryptedURL,
		&value.Payload,
		&value.Attempts,
		&value.MaxAttempts,
		&value.IdempotencyKey,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PendingDelivery{}, ErrNoPendingDelivery
	}
	if err != nil {
		return PendingDelivery{}, fmt.Errorf("claim notification delivery: %w", err)
	}
	if len(encryptedURL) > 0 {
		value.WebhookURL, err = r.vault.DecryptString(encryptedURL, "delivery-webhook")
		if err != nil {
			return PendingDelivery{}, fmt.Errorf("decrypt delivery webhook: %w", err)
		}
	}
	return value, nil
}

func (r *Repository) MarkDeliverySucceeded(ctx context.Context, delivery PendingDelivery, responseStatus int) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin successful delivery: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if _, err := tx.Exec(ctx, `
		UPDATE notification_deliveries
		SET status = 'DELIVERED', response_status = $2, last_error = '',
		    delivered_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, delivery.ID, responseStatus); err != nil {
		return fmt.Errorf("mark notification delivered: %w", err)
	}
	if err := insertEvent(ctx, tx, delivery.IncidentID, "NOTIFICATION_DELIVERED", "Webhook delivered", map[string]any{
		"delivery_id": delivery.ID,
		"event_type":  delivery.EventType,
		"attempt":     delivery.Attempts,
		"status_code": responseStatus,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit successful delivery: %w", err)
	}
	return nil
}

func (r *Repository) MarkDeliveryFailed(
	ctx context.Context,
	delivery PendingDelivery,
	responseStatus int,
	deliveryErr error,
	nextAttempt time.Time,
) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin failed delivery: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	message := deliveryErr.Error()
	if _, err := tx.Exec(ctx, `
		UPDATE notification_deliveries
		SET status = 'FAILED', response_status = $2, last_error = $3,
		    next_attempt_at = $4, updated_at = NOW()
		WHERE id = $1
	`, delivery.ID, responseStatus, message, nextAttempt); err != nil {
		return fmt.Errorf("mark notification failed: %w", err)
	}
	if err := insertEvent(ctx, tx, delivery.IncidentID, "NOTIFICATION_FAILED", message, map[string]any{
		"delivery_id": delivery.ID,
		"event_type":  delivery.EventType,
		"attempt":     delivery.Attempts,
		"status_code": responseStatus,
		"terminal":    delivery.Attempts >= delivery.MaxAttempts,
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit failed delivery: %w", err)
	}
	return nil
}
