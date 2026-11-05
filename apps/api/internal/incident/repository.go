package incident

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db    *pgxpool.Pool
	vault secretVault
}

type secretVault interface {
	EncryptString(string, string) ([]byte, error)
	DecryptString([]byte, string) (string, error)
}

func NewRepository(db *pgxpool.Pool, vault secretVault) *Repository {
	return &Repository{db: db, vault: vault}
}

func (r *Repository) GetAlertRule(ctx context.Context, monitorID string) (AlertRule, error) {
	var rule AlertRule
	var encryptedURL []byte
	var legacyURL string
	err := r.db.QueryRow(ctx, `
		SELECT id::text, monitor_id::text, enabled, webhook_url, webhook_url_encrypted, failure_threshold,
		       failure_window_seconds, latency_threshold_ms, latency_consecutive_checks,
		       cooldown_seconds, notify_recovery, created_at, updated_at
		FROM alert_rules
		WHERE monitor_id = $1
	`, monitorID).Scan(
		&rule.ID,
		&rule.MonitorID,
		&rule.Enabled,
		&legacyURL,
		&encryptedURL,
		&rule.FailureThreshold,
		&rule.FailureWindowSeconds,
		&rule.LatencyThresholdMS,
		&rule.LatencyConsecutiveChecks,
		&rule.CooldownSeconds,
		&rule.NotifyRecovery,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return AlertRule{}, ErrAlertRuleNotFound
	}
	if err != nil {
		return AlertRule{}, fmt.Errorf("get alert rule: %w", err)
	}
	if len(encryptedURL) > 0 {
		rule.WebhookURL, err = r.vault.DecryptString(encryptedURL, "alert-rule-webhook")
		if err != nil {
			return AlertRule{}, fmt.Errorf("decrypt alert webhook: %w", err)
		}
	} else {
		rule.WebhookURL = legacyURL
	}
	rule.WebhookConfigured = rule.WebhookURL != ""
	return rule, nil
}

func (r *Repository) MigrateLegacySecrets(ctx context.Context) error {
	rows, err := r.db.Query(ctx, "SELECT id::text, webhook_url FROM alert_rules WHERE webhook_url <> ''")
	if err != nil {
		return fmt.Errorf("list legacy alert secrets: %w", err)
	}
	type legacySecret struct{ id, value string }
	legacyRules := make([]legacySecret, 0)
	for rows.Next() {
		var item legacySecret
		if err := rows.Scan(&item.id, &item.value); err != nil {
			rows.Close()
			return fmt.Errorf("scan legacy alert secret: %w", err)
		}
		legacyRules = append(legacyRules, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate legacy alert secrets: %w", err)
	}
	rows.Close()
	for _, item := range legacyRules {
		encrypted, err := r.vault.EncryptString(item.value, "alert-rule-webhook")
		if err != nil {
			return fmt.Errorf("encrypt legacy alert secret: %w", err)
		}
		if _, err := r.db.Exec(ctx, `UPDATE alert_rules SET webhook_url_encrypted = $2, webhook_url = '', updated_at = NOW() WHERE id = $1`, item.id, encrypted); err != nil {
			return fmt.Errorf("save encrypted alert secret: %w", err)
		}
	}

	rows, err = r.db.Query(ctx, "SELECT id::text, webhook_url FROM notification_deliveries WHERE webhook_url <> ''")
	if err != nil {
		return fmt.Errorf("list legacy delivery secrets: %w", err)
	}
	legacyDeliveries := make([]legacySecret, 0)
	for rows.Next() {
		var item legacySecret
		if err := rows.Scan(&item.id, &item.value); err != nil {
			rows.Close()
			return fmt.Errorf("scan legacy delivery secret: %w", err)
		}
		legacyDeliveries = append(legacyDeliveries, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate legacy delivery secrets: %w", err)
	}
	rows.Close()
	for _, item := range legacyDeliveries {
		encrypted, err := r.vault.EncryptString(item.value, "delivery-webhook")
		if err != nil {
			return fmt.Errorf("encrypt legacy delivery secret: %w", err)
		}
		if _, err := r.db.Exec(ctx, `UPDATE notification_deliveries SET webhook_url_encrypted = $2, webhook_url = '', updated_at = NOW() WHERE id = $1`, item.id, encrypted); err != nil {
			return fmt.Errorf("save encrypted delivery secret: %w", err)
		}
	}
	return nil
}

func (r *Repository) IsInMaintenance(ctx context.Context, monitorID string, observedAt time.Time) (bool, error) {
	var active bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM maintenance_windows
			WHERE monitor_id = $1
			  AND cancelled_at IS NULL
			  AND starts_at <= $2
			  AND ends_at > $2
		)
	`, monitorID, observedAt).Scan(&active)
	if err != nil {
		return false, fmt.Errorf("check active maintenance: %w", err)
	}
	return active, nil
}

func (r *Repository) FailureWindowReached(
	ctx context.Context,
	monitorID string,
	threshold int,
	windowSeconds int,
	observedAt time.Time,
) (bool, error) {
	var reached bool
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) = $2 AND COUNT(*) FILTER (WHERE outcome = 'DOWN') = $2
		FROM (
			SELECT outcome
			FROM check_results
			WHERE monitor_id = $1
			  AND checked_at <= $4
			  AND checked_at >= $4 - ($3 * INTERVAL '1 second')
			ORDER BY checked_at DESC
			LIMIT $2
		) recent
	`, monitorID, threshold, windowSeconds, observedAt).Scan(&reached)
	if err != nil {
		return false, fmt.Errorf("evaluate consecutive failures: %w", err)
	}
	return reached, nil
}

func (r *Repository) LatencyWindowReached(
	ctx context.Context,
	monitorID string,
	latencyThresholdMS int64,
	consecutiveChecks int,
	observedAt time.Time,
) (bool, error) {
	var reached bool
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) = $3
		   AND COUNT(*) FILTER (WHERE outcome = 'UP' AND response_time_ms >= $2) = $3
		FROM (
			SELECT outcome, response_time_ms
			FROM check_results
			WHERE monitor_id = $1 AND checked_at <= $4
			ORDER BY checked_at DESC
			LIMIT $3
		) recent
	`, monitorID, latencyThresholdMS, consecutiveChecks, observedAt).Scan(&reached)
	if err != nil {
		return false, fmt.Errorf("evaluate consecutive latency checks: %w", err)
	}
	return reached, nil
}

func (r *Repository) OpenIncident(ctx context.Context, trigger Trigger, rule AlertRule) (Incident, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Incident{}, false, fmt.Errorf("begin open incident: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var value Incident
	var created bool
	err = tx.QueryRow(ctx, `
		INSERT INTO incidents (
			monitor_id, kind, status, summary, trigger_check_result_id,
			started_at, last_observation_at
		)
		VALUES ($1, $2, 'OPEN', $3, $4, $5, $5)
		ON CONFLICT (monitor_id, kind) WHERE status <> 'RESOLVED'
		DO UPDATE SET last_observation_at = EXCLUDED.last_observation_at, updated_at = NOW()
		RETURNING id::text, monitor_id::text, kind, status, summary,
		          trigger_check_result_id::text, started_at, last_observation_at,
		          acknowledged_at, acknowledged_by, resolved_at, created_at, updated_at,
		          (xmax = 0)
	`, trigger.MonitorID, trigger.Kind, trigger.Summary, trigger.CheckResultID, trigger.ObservedAt).Scan(
		&value.ID,
		&value.MonitorID,
		&value.Kind,
		&value.Status,
		&value.Summary,
		&value.TriggerCheckResultID,
		&value.StartedAt,
		&value.LastObservationAt,
		&value.AcknowledgedAt,
		&value.AcknowledgedBy,
		&value.ResolvedAt,
		&value.CreatedAt,
		&value.UpdatedAt,
		&created,
	)
	if err != nil {
		return Incident{}, false, fmt.Errorf("upsert active incident: %w", err)
	}

	if created {
		metadata := map[string]any{
			"check_result_id":      trigger.CheckResultID,
			"computed_status":      trigger.ComputedStatus,
			"outcome":              trigger.Outcome,
			"status_code":          trigger.StatusCode,
			"response_time_ms":     trigger.ResponseTimeMS,
			"consecutive_failures": trigger.ConsecutiveFailure,
		}
		if err := insertEvent(ctx, tx, value.ID, "OPENED", trigger.Summary, metadata); err != nil {
			return Incident{}, false, err
		}
		if err := r.queueDelivery(ctx, tx, value, trigger.MonitorName, "INCIDENT_OPENED", rule); err != nil {
			return Incident{}, false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Incident{}, false, fmt.Errorf("commit open incident: %w", err)
	}
	return value, created, nil
}

func (r *Repository) ResolveIncident(
	ctx context.Context,
	monitorID string,
	kind Kind,
	observedAt time.Time,
	rule AlertRule,
	notify bool,
) (Incident, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Incident{}, false, fmt.Errorf("begin resolve incident: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var value Incident
	err = tx.QueryRow(ctx, `
		UPDATE incidents
		SET status = 'RESOLVED', resolved_at = $3, last_observation_at = $3, updated_at = NOW()
		WHERE monitor_id = $1 AND kind = $2 AND status <> 'RESOLVED'
		RETURNING id::text, monitor_id::text, kind, status, summary,
		          trigger_check_result_id::text, started_at, last_observation_at,
		          acknowledged_at, acknowledged_by, resolved_at, created_at, updated_at
	`, monitorID, kind, observedAt).Scan(
		&value.ID,
		&value.MonitorID,
		&value.Kind,
		&value.Status,
		&value.Summary,
		&value.TriggerCheckResultID,
		&value.StartedAt,
		&value.LastObservationAt,
		&value.AcknowledgedAt,
		&value.AcknowledgedBy,
		&value.ResolvedAt,
		&value.CreatedAt,
		&value.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, false, nil
	}
	if err != nil {
		return Incident{}, false, fmt.Errorf("resolve active incident: %w", err)
	}

	message := "Monitor recovered"
	if err := insertEvent(ctx, tx, value.ID, "RESOLVED", message, map[string]any{"resolved_at": observedAt}); err != nil {
		return Incident{}, false, err
	}
	if notify && rule.NotifyRecovery {
		monitorName, err := monitorName(ctx, tx, monitorID)
		if err != nil {
			return Incident{}, false, err
		}
		if err := r.queueDelivery(ctx, tx, value, monitorName, "INCIDENT_RESOLVED", rule); err != nil {
			return Incident{}, false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Incident{}, false, fmt.Errorf("commit resolve incident: %w", err)
	}
	return value, true, nil
}

func (r *Repository) queueDelivery(
	ctx context.Context,
	tx pgx.Tx,
	value Incident,
	monitorName string,
	eventType string,
	rule AlertRule,
) error {
	if rule.WebhookURL == "" {
		return nil
	}
	encryptedURL, err := r.vault.EncryptString(rule.WebhookURL, "delivery-webhook")
	if err != nil {
		return fmt.Errorf("encrypt delivery webhook: %w", err)
	}

	payload, err := json.Marshal(map[string]any{
		"version": "1",
		"event":   eventType,
		"incident": map[string]any{
			"id":          value.ID,
			"kind":        value.Kind,
			"status":      value.Status,
			"summary":     value.Summary,
			"started_at":  value.StartedAt,
			"resolved_at": value.ResolvedAt,
		},
		"monitor": map[string]any{
			"id":   value.MonitorID,
			"name": monitorName,
		},
	})
	if err != nil {
		return fmt.Errorf("encode notification payload: %w", err)
	}

	var nextAttempt time.Time
	err = tx.QueryRow(ctx, `
		SELECT GREATEST(
			NOW(),
			COALESCE(MAX(d.delivered_at) + ($2 * INTERVAL '1 second'), NOW())
		)
		FROM notification_deliveries d
		JOIN incidents i ON i.id = d.incident_id
		WHERE i.monitor_id = $1 AND d.status = 'DELIVERED'
	`, value.MonitorID, rule.CooldownSeconds).Scan(&nextAttempt)
	if err != nil {
		return fmt.Errorf("calculate notification cooldown: %w", err)
	}

	command, err := tx.Exec(ctx, `
		INSERT INTO notification_deliveries (
			incident_id, event_type, webhook_url, webhook_url_encrypted, payload, next_attempt_at, idempotency_key
		)
		VALUES ($1, $2, '', $3, $4, $5, $6)
		ON CONFLICT (idempotency_key) DO NOTHING
	`, value.ID, eventType, encryptedURL, payload, nextAttempt, value.ID+":"+eventType)
	if err != nil {
		return fmt.Errorf("queue notification delivery: %w", err)
	}
	if command.RowsAffected() > 0 {
		if err := insertEvent(ctx, tx, value.ID, "NOTIFICATION_QUEUED", "Webhook delivery queued", map[string]any{"event_type": eventType}); err != nil {
			return err
		}
	}
	return nil
}

func insertEvent(ctx context.Context, tx pgx.Tx, incidentID, eventType, message string, metadata map[string]any) error {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode incident event metadata: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO incident_events (incident_id, event_type, message, metadata)
		VALUES ($1, $2, $3, $4)
	`, incidentID, eventType, message, encoded); err != nil {
		return fmt.Errorf("insert incident event: %w", err)
	}
	return nil
}

func monitorName(ctx context.Context, tx pgx.Tx, monitorID string) (string, error) {
	var name string
	if err := tx.QueryRow(ctx, "SELECT name FROM monitors WHERE id = $1", monitorID).Scan(&name); err != nil {
		return "", fmt.Errorf("get incident monitor name: %w", err)
	}
	return name, nil
}
