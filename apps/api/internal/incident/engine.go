package incident

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/safidy/go-maso/apps/api/internal/monitor"
	"github.com/safidy/go-maso/apps/api/internal/monitoring"
)

var ErrAlertRuleNotFound = errors.New("alert rule not found")

type engineStore interface {
	GetAlertRule(context.Context, string) (AlertRule, error)
	IsInMaintenance(context.Context, string, time.Time) (bool, error)
	FailureWindowReached(context.Context, string, int, int, time.Time) (bool, error)
	LatencyWindowReached(context.Context, string, int64, int, time.Time) (bool, error)
	OpenIncident(context.Context, Trigger, AlertRule) (Incident, bool, error)
	ResolveIncident(context.Context, string, Kind, time.Time, AlertRule, bool) (Incident, bool, error)
}

type Engine struct {
	store engineStore
}

func NewEngine(store engineStore) *Engine {
	return &Engine{store: store}
}

func (e *Engine) Observe(ctx context.Context, target monitor.Monitor, result monitoring.CheckResult) error {
	rule, err := e.store.GetAlertRule(ctx, target.ID)
	if errors.Is(err, ErrAlertRuleNotFound) || (err == nil && !rule.Enabled) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load alert rule: %w", err)
	}

	inMaintenance, err := e.store.IsInMaintenance(ctx, target.ID, result.CheckedAt)
	if err != nil {
		return fmt.Errorf("check maintenance window: %w", err)
	}

	availabilityDown := result.ComputedStatus == monitor.StatusDown && result.ConsecutiveFailures >= rule.FailureThreshold
	if availabilityDown {
		availabilityDown, err = e.store.FailureWindowReached(
			ctx,
			target.ID,
			rule.FailureThreshold,
			rule.FailureWindowSeconds,
			result.CheckedAt,
		)
		if err != nil {
			return fmt.Errorf("evaluate failure window: %w", err)
		}
	}

	latencyHigh := rule.LatencyThresholdMS > 0 &&
		result.Outcome == monitoring.OutcomeUp &&
		result.ResponseTimeMS >= rule.LatencyThresholdMS
	if latencyHigh {
		latencyHigh, err = e.store.LatencyWindowReached(
			ctx,
			target.ID,
			rule.LatencyThresholdMS,
			rule.LatencyConsecutiveChecks,
			result.CheckedAt,
		)
		if err != nil {
			return fmt.Errorf("evaluate latency window: %w", err)
		}
	}

	if availabilityDown && !inMaintenance {
		_, _, err = e.store.OpenIncident(ctx, Trigger{
			CheckResultID:      result.ID,
			MonitorID:          target.ID,
			MonitorName:        target.Name,
			Kind:               KindAvailability,
			Summary:            fmt.Sprintf("%s is unavailable", target.Name),
			ComputedStatus:     string(result.ComputedStatus),
			Outcome:            string(result.Outcome),
			StatusCode:         result.StatusCode,
			ResponseTimeMS:     result.ResponseTimeMS,
			ConsecutiveFailure: result.ConsecutiveFailures,
			ObservedAt:         result.CheckedAt,
		}, rule)
		if err != nil {
			return fmt.Errorf("open availability incident: %w", err)
		}
	} else if result.Outcome == monitoring.OutcomeUp {
		if _, _, err := e.store.ResolveIncident(ctx, target.ID, KindAvailability, result.CheckedAt, rule, !inMaintenance); err != nil {
			return fmt.Errorf("resolve availability incident: %w", err)
		}
	}

	if latencyHigh && !inMaintenance {
		_, _, err = e.store.OpenIncident(ctx, Trigger{
			CheckResultID:  result.ID,
			MonitorID:      target.ID,
			MonitorName:    target.Name,
			Kind:           KindLatency,
			Summary:        fmt.Sprintf("%s latency exceeded %d ms", target.Name, rule.LatencyThresholdMS),
			ComputedStatus: string(result.ComputedStatus),
			Outcome:        string(result.Outcome),
			StatusCode:     result.StatusCode,
			ResponseTimeMS: result.ResponseTimeMS,
			ObservedAt:     result.CheckedAt,
		}, rule)
		if err != nil {
			return fmt.Errorf("open latency incident: %w", err)
		}
	} else {
		if _, _, err := e.store.ResolveIncident(ctx, target.ID, KindLatency, result.CheckedAt, rule, !inMaintenance); err != nil {
			return fmt.Errorf("resolve latency incident: %w", err)
		}
	}

	return nil
}
