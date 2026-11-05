package incident

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/safidy/go-maso/apps/api/internal/monitor"
	"github.com/safidy/go-maso/apps/api/internal/monitoring"
	"github.com/safidy/go-maso/apps/api/internal/securestore"
)

func TestIncidentLifecycleAndUniqueWebhookAgainstPostgres(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create database pool: %v", err)
	}
	defer pool.Close()

	var deliveries atomic.Int32
	webhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") == "" {
			t.Error("webhook request has no idempotency key")
		}
		deliveries.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer webhook.Close()

	var applicationID, environmentID, monitorID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO applications (name, owner) VALUES ('Incident integration test', 'tests')
		RETURNING id::text
	`).Scan(&applicationID); err != nil {
		t.Fatalf("insert application: %v", err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM applications WHERE id = $1", applicationID)
	}()
	if err := pool.QueryRow(ctx, `
		INSERT INTO environments (application_id, name, base_url)
		VALUES ($1, 'test', 'https://example.com') RETURNING id::text
	`, applicationID).Scan(&environmentID); err != nil {
		t.Fatalf("insert environment: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO monitors (environment_id, name, url, failure_threshold)
		VALUES ($1, 'Lifecycle monitor', 'https://example.com/health', 2)
		RETURNING id::text
	`, environmentID).Scan(&monitorID); err != nil {
		t.Fatalf("insert monitor: %v", err)
	}

	vault, err := securestore.NewVault(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatalf("create test secret vault: %v", err)
	}
	repository := NewRepository(pool, vault)
	_, err = repository.UpsertAlertRule(ctx, monitorID, AlertRuleInput{
		Enabled:                  true,
		WebhookURL:               webhook.URL,
		FailureThreshold:         2,
		FailureWindowSeconds:     300,
		LatencyConsecutiveChecks: 2,
		CooldownSeconds:          0,
		NotifyRecovery:           true,
	})
	if err != nil {
		t.Fatalf("upsert alert rule: %v", err)
	}

	target := monitor.Monitor{ID: monitorID, Name: "Lifecycle monitor"}
	checks := monitoring.NewRepository(pool)
	engine := NewEngine(repository)
	record := func(outcome monitoring.Outcome) monitoring.CheckResult {
		t.Helper()
		result, err := checks.Record(ctx, monitoring.CheckResult{MonitorID: monitorID, Outcome: outcome, ResponseTimeMS: 20})
		if err != nil {
			t.Fatalf("record %s check: %v", outcome, err)
		}
		if err := engine.Observe(ctx, target, result); err != nil {
			t.Fatalf("observe %s check: %v", outcome, err)
		}
		return result
	}

	record(monitoring.OutcomeDown)
	record(monitoring.OutcomeDown)
	record(monitoring.OutcomeDown)

	var activeIncidents, openingDeliveries int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM incidents WHERE monitor_id = $1 AND status <> 'RESOLVED'", monitorID).Scan(&activeIncidents); err != nil {
		t.Fatalf("count active incidents: %v", err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_deliveries d JOIN incidents i ON i.id = d.incident_id WHERE i.monitor_id = $1 AND d.event_type = 'INCIDENT_OPENED'", monitorID).Scan(&openingDeliveries); err != nil {
		t.Fatalf("count opening deliveries: %v", err)
	}
	if activeIncidents != 1 || openingDeliveries != 1 {
		t.Fatalf("active incidents = %d, opening deliveries = %d, want 1 and 1", activeIncidents, openingDeliveries)
	}

	dispatcher := NewDispatcher(repository, NewWebhookSender(webhook.Client()))
	if err := dispatcher.deliverOne(ctx); err != nil {
		t.Fatalf("deliver opening webhook: %v", err)
	}
	record(monitoring.OutcomeUp)
	if err := dispatcher.deliverOne(ctx); err != nil {
		t.Fatalf("deliver recovery webhook: %v", err)
	}

	var resolvedIncidents int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM incidents WHERE monitor_id = $1 AND status = 'RESOLVED'", monitorID).Scan(&resolvedIncidents); err != nil {
		t.Fatalf("count resolved incidents: %v", err)
	}
	if resolvedIncidents != 1 || deliveries.Load() != 2 {
		t.Fatalf("resolved incidents = %d, webhook deliveries = %d, want 1 and 2", resolvedIncidents, deliveries.Load())
	}
}
