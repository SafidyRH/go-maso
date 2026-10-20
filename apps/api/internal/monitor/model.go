package monitor

import "time"

type Status string

const (
	StatusUp       Status = "UP"
	StatusDown     Status = "DOWN"
	StatusDegraded Status = "DEGRADED"
	StatusUnknown  Status = "UNKNOWN"
)

type Monitor struct {
	ID                   string     `json:"id"`
	EnvironmentID        string     `json:"environment_id"`
	Name                 string     `json:"name"`
	URL                  string     `json:"url"`
	Method               string     `json:"method"`
	CheckIntervalSeconds int        `json:"check_interval_seconds"`
	TimeoutMS            int        `json:"timeout_ms"`
	ExpectedStatusMin    int        `json:"expected_status_min"`
	ExpectedStatusMax    int        `json:"expected_status_max"`
	FailureThreshold     int        `json:"failure_threshold"`
	Enabled              bool       `json:"enabled"`
	EffectiveEnabled     bool       `json:"effective_enabled"`
	CurrentStatus        Status     `json:"current_status"`
	ConsecutiveFailures  int        `json:"consecutive_failures"`
	LastCheckedAt        *time.Time `json:"last_checked_at,omitempty"`
	ArchivedAt           *time.Time `json:"archived_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type CreateMonitorInput struct {
	Name                 string `json:"name"`
	URL                  string `json:"url"`
	Method               string `json:"method"`
	CheckIntervalSeconds int    `json:"check_interval_seconds"`
	TimeoutMS            int    `json:"timeout_ms"`
	ExpectedStatusMin    int    `json:"expected_status_min"`
	ExpectedStatusMax    int    `json:"expected_status_max"`
	FailureThreshold     int    `json:"failure_threshold"`
	Enabled              *bool  `json:"enabled"`
}

type UpdateMonitorInput struct {
	Name                 *string `json:"name"`
	URL                  *string `json:"url"`
	Method               *string `json:"method"`
	CheckIntervalSeconds *int    `json:"check_interval_seconds"`
	TimeoutMS            *int    `json:"timeout_ms"`
	ExpectedStatusMin    *int    `json:"expected_status_min"`
	ExpectedStatusMax    *int    `json:"expected_status_max"`
	FailureThreshold     *int    `json:"failure_threshold"`
	Enabled              *bool   `json:"enabled"`
}

type Page struct {
	Items  []Monitor `json:"items"`
	Limit  int       `json:"limit"`
	Offset int       `json:"offset"`
}
