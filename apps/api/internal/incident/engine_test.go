package incident

import (
	"context"
	"testing"
	"time"

	"github.com/safidy/go-maso/apps/api/internal/monitor"
	"github.com/safidy/go-maso/apps/api/internal/monitoring"
)

type fakeEngineStore struct {
	rule                  AlertRule
	ruleErr               error
	maintenance           bool
	failureWindowReached  bool
	latencyWindowReached  bool
	opened                []Kind
	resolved              []Kind
	recoveryNotifications []bool
}

func (s *fakeEngineStore) GetAlertRule(context.Context, string) (AlertRule, error) {
	return s.rule, s.ruleErr
}

func (s *fakeEngineStore) IsInMaintenance(context.Context, string, time.Time) (bool, error) {
	return s.maintenance, nil
}

func (s *fakeEngineStore) FailureWindowReached(context.Context, string, int, int, time.Time) (bool, error) {
	return s.failureWindowReached, nil
}

func (s *fakeEngineStore) LatencyWindowReached(context.Context, string, int64, int, time.Time) (bool, error) {
	return s.latencyWindowReached, nil
}

func (s *fakeEngineStore) OpenIncident(_ context.Context, trigger Trigger, _ AlertRule) (Incident, bool, error) {
	s.opened = append(s.opened, trigger.Kind)
	return Incident{Kind: trigger.Kind}, true, nil
}

func (s *fakeEngineStore) ResolveIncident(_ context.Context, _ string, kind Kind, _ time.Time, _ AlertRule, notify bool) (Incident, bool, error) {
	s.resolved = append(s.resolved, kind)
	s.recoveryNotifications = append(s.recoveryNotifications, notify)
	return Incident{Kind: kind}, true, nil
}

func enabledRule() AlertRule {
	return AlertRule{
		Enabled:                  true,
		WebhookURL:               "https://hooks.example.com/go-maso",
		FailureThreshold:         3,
		FailureWindowSeconds:     300,
		LatencyThresholdMS:       500,
		LatencyConsecutiveChecks: 2,
		NotifyRecovery:           true,
	}
}

func testMonitor() monitor.Monitor {
	return monitor.Monitor{ID: "monitor-id", Name: "Public API"}
}

func TestEngineOpensAvailabilityIncidentAfterConfirmedFailure(t *testing.T) {
	t.Parallel()

	store := &fakeEngineStore{rule: enabledRule(), failureWindowReached: true}
	result := monitoring.CheckResult{
		ID:                  "check-id",
		Outcome:             monitoring.OutcomeDown,
		ComputedStatus:      monitor.StatusDown,
		ConsecutiveFailures: 3,
		CheckedAt:           time.Now().UTC(),
	}

	if err := NewEngine(store).Observe(context.Background(), testMonitor(), result); err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if len(store.opened) != 1 || store.opened[0] != KindAvailability {
		t.Fatalf("opened incidents = %v, want [AVAILABILITY]", store.opened)
	}
}

func TestEngineSuppressesIncidentDuringMaintenance(t *testing.T) {
	t.Parallel()

	store := &fakeEngineStore{rule: enabledRule(), maintenance: true, failureWindowReached: true}
	result := monitoring.CheckResult{
		Outcome:             monitoring.OutcomeDown,
		ComputedStatus:      monitor.StatusDown,
		ConsecutiveFailures: 3,
		CheckedAt:           time.Now().UTC(),
	}

	if err := NewEngine(store).Observe(context.Background(), testMonitor(), result); err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if len(store.opened) != 0 {
		t.Fatalf("opened incidents = %v, want none", store.opened)
	}
}

func TestEngineOpensLatencyIncidentAfterConsecutiveSlowChecks(t *testing.T) {
	t.Parallel()

	store := &fakeEngineStore{rule: enabledRule(), latencyWindowReached: true}
	result := monitoring.CheckResult{
		Outcome:        monitoring.OutcomeUp,
		ComputedStatus: monitor.StatusUp,
		ResponseTimeMS: 750,
		CheckedAt:      time.Now().UTC(),
	}

	if err := NewEngine(store).Observe(context.Background(), testMonitor(), result); err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if len(store.opened) != 1 || store.opened[0] != KindLatency {
		t.Fatalf("opened incidents = %v, want [LATENCY]", store.opened)
	}
}

func TestEngineResolvesIncidentsOnHealthyObservation(t *testing.T) {
	t.Parallel()

	store := &fakeEngineStore{rule: enabledRule()}
	result := monitoring.CheckResult{
		Outcome:        monitoring.OutcomeUp,
		ComputedStatus: monitor.StatusUp,
		ResponseTimeMS: 120,
		CheckedAt:      time.Now().UTC(),
	}

	if err := NewEngine(store).Observe(context.Background(), testMonitor(), result); err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if len(store.resolved) != 2 || store.resolved[0] != KindAvailability || store.resolved[1] != KindLatency {
		t.Fatalf("resolved incidents = %v, want [AVAILABILITY LATENCY]", store.resolved)
	}
}

func TestEngineIgnoresMonitorWithoutEnabledRule(t *testing.T) {
	t.Parallel()

	store := &fakeEngineStore{ruleErr: ErrAlertRuleNotFound}
	if err := NewEngine(store).Observe(context.Background(), testMonitor(), monitoring.CheckResult{}); err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if len(store.opened) != 0 || len(store.resolved) != 0 {
		t.Fatalf("unexpected incident actions: opened=%v resolved=%v", store.opened, store.resolved)
	}
}
