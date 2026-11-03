package incident

import "time"

type Kind string

const (
	KindAvailability Kind = "AVAILABILITY"
	KindLatency      Kind = "LATENCY"
)

type Status string

const (
	StatusOpen         Status = "OPEN"
	StatusAcknowledged Status = "ACKNOWLEDGED"
	StatusResolved     Status = "RESOLVED"
)

type AlertRule struct {
	ID                       string    `json:"id"`
	MonitorID                string    `json:"monitor_id"`
	Enabled                  bool      `json:"enabled"`
	WebhookURL               string    `json:"webhook_url"`
	FailureThreshold         int       `json:"failure_threshold"`
	FailureWindowSeconds     int       `json:"failure_window_seconds"`
	LatencyThresholdMS       int64     `json:"latency_threshold_ms"`
	LatencyConsecutiveChecks int       `json:"latency_consecutive_checks"`
	CooldownSeconds          int       `json:"cooldown_seconds"`
	NotifyRecovery           bool      `json:"notify_recovery"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type AlertRuleInput struct {
	Enabled                  bool   `json:"enabled"`
	WebhookURL               string `json:"webhook_url"`
	FailureThreshold         int    `json:"failure_threshold"`
	FailureWindowSeconds     int    `json:"failure_window_seconds"`
	LatencyThresholdMS       int64  `json:"latency_threshold_ms"`
	LatencyConsecutiveChecks int    `json:"latency_consecutive_checks"`
	CooldownSeconds          int    `json:"cooldown_seconds"`
	NotifyRecovery           bool   `json:"notify_recovery"`
}

type Incident struct {
	ID                   string     `json:"id"`
	MonitorID            string     `json:"monitor_id"`
	MonitorName          string     `json:"monitor_name,omitempty"`
	EnvironmentName      string     `json:"environment_name,omitempty"`
	ApplicationID        string     `json:"application_id,omitempty"`
	ApplicationName      string     `json:"application_name,omitempty"`
	Kind                 Kind       `json:"kind"`
	Status               Status     `json:"status"`
	Summary              string     `json:"summary"`
	TriggerCheckResultID *string    `json:"trigger_check_result_id,omitempty"`
	StartedAt            time.Time  `json:"started_at"`
	LastObservationAt    time.Time  `json:"last_observation_at"`
	AcknowledgedAt       *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy       string     `json:"acknowledged_by,omitempty"`
	ResolvedAt           *time.Time `json:"resolved_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type Event struct {
	ID         string         `json:"id"`
	IncidentID string         `json:"incident_id"`
	EventType  string         `json:"event_type"`
	Message    string         `json:"message"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  time.Time      `json:"created_at"`
}

type MaintenanceWindow struct {
	ID          string     `json:"id"`
	MonitorID   string     `json:"monitor_id"`
	Reason      string     `json:"reason"`
	StartsAt    time.Time  `json:"starts_at"`
	EndsAt      time.Time  `json:"ends_at"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type CreateMaintenanceInput struct {
	Reason   string    `json:"reason"`
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}

type Delivery struct {
	ID             string     `json:"id"`
	IncidentID     string     `json:"incident_id"`
	EventType      string     `json:"event_type"`
	Channel        string     `json:"channel"`
	Status         string     `json:"status"`
	Attempts       int        `json:"attempts"`
	MaxAttempts    int        `json:"max_attempts"`
	NextAttemptAt  time.Time  `json:"next_attempt_at"`
	ResponseStatus int        `json:"response_status"`
	LastError      string     `json:"last_error,omitempty"`
	DeliveredAt    *time.Time `json:"delivered_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type Trigger struct {
	CheckResultID      string
	MonitorID          string
	MonitorName        string
	Kind               Kind
	Summary            string
	ComputedStatus     string
	Outcome            string
	StatusCode         int
	ResponseTimeMS     int64
	ConsecutiveFailure int
	ObservedAt         time.Time
}
