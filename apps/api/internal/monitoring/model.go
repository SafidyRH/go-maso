package monitoring

import "time"

type Status string

const (
	StatusUp   Status = "UP"
	StatusDown Status = "DOWN"
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
