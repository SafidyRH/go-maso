package monitoring

import (
	"time"

	"github.com/safidy/go-maso/apps/api/internal/monitor"
)

type Outcome string

const (
	OutcomeUp   Outcome = "UP"
	OutcomeDown Outcome = "DOWN"
)

type CheckResult struct {
	ID                  string         `json:"id"`
	MonitorID           string         `json:"monitor_id"`
	Outcome             Outcome        `json:"outcome"`
	ComputedStatus      monitor.Status `json:"computed_status"`
	ConsecutiveFailures int            `json:"consecutive_failures"`
	StatusCode          int            `json:"status_code"`
	ResponseTimeMS      int64          `json:"response_time_ms"`
	ErrorMessage        string         `json:"error_message,omitempty"`
	CheckedAt           time.Time      `json:"checked_at"`
}

type Summary struct {
	MonitorID           string         `json:"monitor_id"`
	Status              monitor.Status `json:"status"`
	ConsecutiveFailures int            `json:"consecutive_failures"`
	LastStatusCode      int            `json:"last_status_code"`
	LastResponseTimeMS  int64          `json:"last_response_time_ms"`
	LastCheckedAt       *time.Time     `json:"last_checked_at"`

	Uptime24H             float64 `json:"uptime_24h"`
	AverageResponseTimeMS float64 `json:"average_response_time_ms"`
	TotalChecks24H        int64   `json:"total_checks_24h"`
	UpChecks24H           int64   `json:"up_checks_24h"`
	DownChecks24H         int64   `json:"down_checks_24h"`
}
