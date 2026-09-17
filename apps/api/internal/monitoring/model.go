package monitoring

import "time"

type Status string

const (
	StatusUp      Status = "UP"
	StatusDown    Status = "DOWN"
	StatusUnknown Status = "UNKNOWN"
)

type HealthCheck struct {
	ID             string    `json:"id"`
	ApplicationID  string    `json:"application_id"`
	Status         Status    `json:"status"`
	StatusCode     int       `json:"status_code"`
	ResponseTimeMS int64     `json:"response_time_ms"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	CheckedAt      time.Time `json:"checked_at"`
}

type Summary struct {
	ApplicationID      string     `json:"application_id"`
	Status             Status     `json:"status"`
	LastStatusCode     int        `json:"last_status_code"`
	LastResponseTimeMS int64      `json:"last_response_time_ms"`
	LastCheckedAt      *time.Time `json:"last_checked_at"`

	Uptime24H             float64 `json:"uptime_24h"`
	AverageResponseTimeMS float64 `json:average_response_time_ms`

	TotalChecks24H int64 `json:"total_checks_24h"`
	UpChecks24H    int64 `json:"up_checks_24h"`
	DownChecks24H  int64 `json:"down_checks_24h"`
}
