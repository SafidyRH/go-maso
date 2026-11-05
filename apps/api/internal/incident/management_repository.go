package incident

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrIncidentNotFound    = errors.New("incident not found")
	ErrIncidentResolved    = errors.New("incident is already resolved")
	ErrMaintenanceNotFound = errors.New("maintenance window not found")
)

type IncidentFilter struct {
	ApplicationID string
	MonitorID     string
	Status        Status
	Limit         int
}

func DefaultAlertRule(monitorID string) AlertRule {
	return AlertRule{
		MonitorID:                monitorID,
		FailureThreshold:         3,
		FailureWindowSeconds:     300,
		LatencyConsecutiveChecks: 3,
		CooldownSeconds:          300,
		NotifyRecovery:           true,
	}
}

func (r *Repository) UpsertAlertRule(ctx context.Context, monitorID string, input AlertRuleInput) (AlertRule, error) {
	var encryptedURL []byte
	var err error
	if input.WebhookURL != "" {
		encryptedURL, err = r.vault.EncryptString(input.WebhookURL, "alert-rule-webhook")
		if err != nil {
			return AlertRule{}, fmt.Errorf("encrypt alert webhook: %w", err)
		}
	}
	generatedSigningSecret, err := generateSigningSecret()
	if err != nil {
		return AlertRule{}, err
	}
	encryptedSigningSecret, err := r.vault.EncryptString(generatedSigningSecret, "alert-rule-signing")
	if err != nil {
		return AlertRule{}, fmt.Errorf("encrypt webhook signing secret: %w", err)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return AlertRule{}, fmt.Errorf("begin alert rule update: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var rule AlertRule
	var storedEncryptedURL []byte
	var storedEncryptedSigningSecret []byte
	var created bool
	err = tx.QueryRow(ctx, `
		INSERT INTO alert_rules (
			monitor_id, enabled, webhook_url, webhook_url_encrypted, signing_secret_encrypted,
			failure_threshold, failure_window_seconds,
			latency_threshold_ms, latency_consecutive_checks, cooldown_seconds, notify_recovery
		)
		VALUES ($1, $2, '', COALESCE($3::bytea, ''::bytea), $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (monitor_id) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			webhook_url = '',
			webhook_url_encrypted = CASE
				WHEN $3::bytea IS NULL THEN alert_rules.webhook_url_encrypted
				ELSE $3
			END,
			signing_secret_encrypted = CASE
				WHEN OCTET_LENGTH(alert_rules.signing_secret_encrypted) = 0 THEN EXCLUDED.signing_secret_encrypted
				ELSE alert_rules.signing_secret_encrypted
			END,
			failure_threshold = EXCLUDED.failure_threshold,
			failure_window_seconds = EXCLUDED.failure_window_seconds,
			latency_threshold_ms = EXCLUDED.latency_threshold_ms,
			latency_consecutive_checks = EXCLUDED.latency_consecutive_checks,
			cooldown_seconds = EXCLUDED.cooldown_seconds,
			notify_recovery = EXCLUDED.notify_recovery,
			updated_at = NOW()
		RETURNING id::text, monitor_id::text, enabled, webhook_url_encrypted, signing_secret_encrypted, failure_threshold,
		          failure_window_seconds, latency_threshold_ms, latency_consecutive_checks,
		          cooldown_seconds, notify_recovery, created_at, updated_at, (xmax = 0)
	`,
		monitorID,
		input.Enabled,
		encryptedURL,
		encryptedSigningSecret,
		input.FailureThreshold,
		input.FailureWindowSeconds,
		input.LatencyThresholdMS,
		input.LatencyConsecutiveChecks,
		input.CooldownSeconds,
		input.NotifyRecovery,
	).Scan(
		&rule.ID,
		&rule.MonitorID,
		&rule.Enabled,
		&storedEncryptedURL,
		&storedEncryptedSigningSecret,
		&rule.FailureThreshold,
		&rule.FailureWindowSeconds,
		&rule.LatencyThresholdMS,
		&rule.LatencyConsecutiveChecks,
		&rule.CooldownSeconds,
		&rule.NotifyRecovery,
		&rule.CreatedAt,
		&rule.UpdatedAt,
		&created,
	)
	if err != nil {
		return AlertRule{}, fmt.Errorf("upsert alert rule: %w", err)
	}
	if len(storedEncryptedURL) > 0 {
		rule.WebhookURL, err = r.vault.DecryptString(storedEncryptedURL, "alert-rule-webhook")
		if err != nil {
			return AlertRule{}, fmt.Errorf("decrypt saved alert webhook: %w", err)
		}
	}
	rule.WebhookConfigured = rule.WebhookURL != ""
	if len(storedEncryptedSigningSecret) > 0 {
		rule.SigningSecret, err = r.vault.DecryptString(storedEncryptedSigningSecret, "alert-rule-signing")
		if err != nil {
			return AlertRule{}, fmt.Errorf("decrypt saved webhook signing secret: %w", err)
		}
	}
	rule.SigningSecretConfigured = rule.SigningSecret != ""
	if created {
		rule.RevealedSigningSecret = rule.SigningSecret
	}

	if _, err := tx.Exec(ctx, `
		UPDATE monitors SET failure_threshold = $2, updated_at = NOW()
		WHERE id = $1 AND archived_at IS NULL
	`, monitorID, input.FailureThreshold); err != nil {
		return AlertRule{}, fmt.Errorf("synchronize monitor failure threshold: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return AlertRule{}, fmt.Errorf("commit alert rule update: %w", err)
	}
	return rule, nil
}

func (r *Repository) RotateSigningSecret(ctx context.Context, monitorID string) (SigningSecretRotation, error) {
	secret, err := generateSigningSecret()
	if err != nil {
		return SigningSecretRotation{}, err
	}
	encrypted, err := r.vault.EncryptString(secret, "alert-rule-signing")
	if err != nil {
		return SigningSecretRotation{}, fmt.Errorf("encrypt webhook signing secret: %w", err)
	}
	var rotatedAt time.Time
	err = r.db.QueryRow(ctx, `
		UPDATE alert_rules
		SET signing_secret_encrypted = $2, updated_at = NOW()
		WHERE monitor_id = $1
		RETURNING updated_at
	`, monitorID, encrypted).Scan(&rotatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SigningSecretRotation{}, ErrAlertRuleNotFound
	}
	if err != nil {
		return SigningSecretRotation{}, fmt.Errorf("rotate webhook signing secret: %w", err)
	}
	return SigningSecretRotation{SigningSecret: secret, RotatedAt: rotatedAt}, nil
}

func (r *Repository) ListIncidents(ctx context.Context, filter IncidentFilter) ([]Incident, error) {
	rows, err := r.db.Query(ctx, `
		SELECT i.id::text, i.monitor_id::text, m.name, e.name, a.id::text, a.name,
		       i.kind, i.status, i.summary, i.trigger_check_result_id::text,
		       i.started_at, i.last_observation_at, i.acknowledged_at,
		       i.acknowledged_by, i.resolved_at, i.created_at, i.updated_at
		FROM incidents i
		JOIN monitors m ON m.id = i.monitor_id
		JOIN environments e ON e.id = m.environment_id
		JOIN applications a ON a.id = e.application_id
		WHERE ($1 = '' OR a.id::text = $1)
		  AND ($2 = '' OR m.id::text = $2)
		  AND ($3 = '' OR i.status = $3)
		ORDER BY CASE WHEN i.status = 'RESOLVED' THEN 1 ELSE 0 END, i.started_at DESC
		LIMIT $4
	`, filter.ApplicationID, filter.MonitorID, filter.Status, filter.Limit)
	if err != nil {
		return nil, fmt.Errorf("list incidents: %w", err)
	}
	defer rows.Close()

	items := make([]Incident, 0)
	for rows.Next() {
		item, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incidents: %w", err)
	}
	return items, nil
}

func (r *Repository) GetIncident(ctx context.Context, id string) (Incident, error) {
	item, err := scanIncident(r.db.QueryRow(ctx, `
		SELECT i.id::text, i.monitor_id::text, m.name, e.name, a.id::text, a.name,
		       i.kind, i.status, i.summary, i.trigger_check_result_id::text,
		       i.started_at, i.last_observation_at, i.acknowledged_at,
		       i.acknowledged_by, i.resolved_at, i.created_at, i.updated_at
		FROM incidents i
		JOIN monitors m ON m.id = i.monitor_id
		JOIN environments e ON e.id = m.environment_id
		JOIN applications a ON a.id = e.application_id
		WHERE i.id = $1
	`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Incident{}, ErrIncidentNotFound
	}
	return item, err
}

func (r *Repository) Acknowledge(ctx context.Context, id, acknowledgedBy string) (Incident, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Incident{}, fmt.Errorf("begin incident acknowledgement: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var incidentID string
	err = tx.QueryRow(ctx, `
		UPDATE incidents
		SET status = 'ACKNOWLEDGED', acknowledged_at = COALESCE(acknowledged_at, NOW()),
		    acknowledged_by = CASE WHEN acknowledged_by = '' THEN $2 ELSE acknowledged_by END,
		    updated_at = NOW()
		WHERE id = $1 AND status <> 'RESOLVED'
		RETURNING id::text
	`, id, acknowledgedBy).Scan(&incidentID)
	if errors.Is(err, pgx.ErrNoRows) {
		var status Status
		lookupErr := tx.QueryRow(ctx, "SELECT status FROM incidents WHERE id = $1", id).Scan(&status)
		if errors.Is(lookupErr, pgx.ErrNoRows) {
			return Incident{}, ErrIncidentNotFound
		}
		if lookupErr != nil {
			return Incident{}, fmt.Errorf("lookup incident acknowledgement: %w", lookupErr)
		}
		return Incident{}, ErrIncidentResolved
	}
	if err != nil {
		return Incident{}, fmt.Errorf("acknowledge incident: %w", err)
	}

	if err := insertEvent(ctx, tx, incidentID, "ACKNOWLEDGED", "Incident acknowledged by "+acknowledgedBy, map[string]any{"acknowledged_by": acknowledgedBy}); err != nil {
		return Incident{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Incident{}, fmt.Errorf("commit incident acknowledgement: %w", err)
	}
	return r.GetIncident(ctx, id)
}

func (r *Repository) ListEvents(ctx context.Context, incidentID string) ([]Event, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, incident_id::text, event_type, message, metadata, created_at
		FROM incident_events
		WHERE incident_id = $1
		ORDER BY created_at DESC
	`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("list incident events: %w", err)
	}
	defer rows.Close()

	items := make([]Event, 0)
	for rows.Next() {
		var item Event
		var metadata []byte
		if err := rows.Scan(&item.ID, &item.IncidentID, &item.EventType, &item.Message, &metadata, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan incident event: %w", err)
		}
		if err := json.Unmarshal(metadata, &item.Metadata); err != nil {
			return nil, fmt.Errorf("decode incident event metadata: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate incident events: %w", err)
	}
	return items, nil
}

func (r *Repository) ListDeliveries(ctx context.Context, incidentID string) ([]Delivery, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, incident_id::text, event_type, channel, status, attempts,
		       max_attempts, next_attempt_at, response_status, last_error,
		       delivered_at, created_at, updated_at
		FROM notification_deliveries
		WHERE incident_id = $1
		ORDER BY created_at DESC
	`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("list notification deliveries: %w", err)
	}
	defer rows.Close()

	items := make([]Delivery, 0)
	for rows.Next() {
		var item Delivery
		if err := rows.Scan(
			&item.ID, &item.IncidentID, &item.EventType, &item.Channel, &item.Status,
			&item.Attempts, &item.MaxAttempts, &item.NextAttemptAt, &item.ResponseStatus,
			&item.LastError, &item.DeliveredAt, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan notification delivery: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notification deliveries: %w", err)
	}
	return items, nil
}

func (r *Repository) CreateMaintenance(ctx context.Context, monitorID string, input CreateMaintenanceInput) (MaintenanceWindow, error) {
	var item MaintenanceWindow
	err := r.db.QueryRow(ctx, `
		INSERT INTO maintenance_windows (monitor_id, reason, starts_at, ends_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id::text, monitor_id::text, reason, starts_at, ends_at, cancelled_at, created_at
	`, monitorID, input.Reason, input.StartsAt, input.EndsAt).Scan(
		&item.ID, &item.MonitorID, &item.Reason, &item.StartsAt, &item.EndsAt,
		&item.CancelledAt, &item.CreatedAt,
	)
	if err != nil {
		return MaintenanceWindow{}, fmt.Errorf("create maintenance window: %w", err)
	}
	return item, nil
}

func (r *Repository) ListMaintenances(ctx context.Context, monitorID string) ([]MaintenanceWindow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, monitor_id::text, reason, starts_at, ends_at, cancelled_at, created_at
		FROM maintenance_windows
		WHERE monitor_id = $1
		  AND cancelled_at IS NULL
		  AND ends_at > NOW() - INTERVAL '24 hours'
		ORDER BY starts_at
	`, monitorID)
	if err != nil {
		return nil, fmt.Errorf("list maintenance windows: %w", err)
	}
	defer rows.Close()

	items := make([]MaintenanceWindow, 0)
	for rows.Next() {
		var item MaintenanceWindow
		if err := rows.Scan(&item.ID, &item.MonitorID, &item.Reason, &item.StartsAt, &item.EndsAt, &item.CancelledAt, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan maintenance window: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate maintenance windows: %w", err)
	}
	return items, nil
}

func (r *Repository) CancelMaintenance(ctx context.Context, id string) error {
	command, err := r.db.Exec(ctx, `
		UPDATE maintenance_windows SET cancelled_at = NOW()
		WHERE id = $1 AND cancelled_at IS NULL
	`, id)
	if err != nil {
		return fmt.Errorf("cancel maintenance window: %w", err)
	}
	if command.RowsAffected() == 0 {
		return ErrMaintenanceNotFound
	}
	return nil
}

type incidentScanner interface {
	Scan(...any) error
}

func scanIncident(row incidentScanner) (Incident, error) {
	var item Incident
	err := row.Scan(
		&item.ID,
		&item.MonitorID,
		&item.MonitorName,
		&item.EnvironmentName,
		&item.ApplicationID,
		&item.ApplicationName,
		&item.Kind,
		&item.Status,
		&item.Summary,
		&item.TriggerCheckResultID,
		&item.StartedAt,
		&item.LastObservationAt,
		&item.AcknowledgedAt,
		&item.AcknowledgedBy,
		&item.ResolvedAt,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return Incident{}, fmt.Errorf("scan incident: %w", err)
	}
	return item, nil
}
