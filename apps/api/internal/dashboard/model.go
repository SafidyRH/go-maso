package dashboard

import (
	"time"

	"github.com/safidy/go-maso/apps/api/internal/monitor"
)

type ApplicationOverview struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Owner    string         `json:"owner"`
	Status   monitor.Status `json:"status"`
	Monitors int64          `json:"monitors"`

	LastStatusCode     int        `json:"last_status_code"`
	LastResponseTimeMS int64      `json:"last_response_time_ms"`
	LastCheckedAt      *time.Time `json:"last_checked_at,omitempty"`

	Uptime24H             float64 `json:"uptime_24h"`
	AverageResponseTimeMS float64 `json:"average_response_time_ms"`
	TotalChecks24H        int64   `json:"total_checks_24h"`
	UpChecks24H           int64   `json:"up_checks_24h"`
	DownChecks24H         int64   `json:"down_checks_24h"`
}

type ApplicationDetail struct {
	ApplicationOverview
	PeriodHours   int                 `json:"period_hours"`
	PeriodSummary PeriodSummary       `json:"period_summary"`
	GeneratedAt   time.Time           `json:"generated_at"`
	Environments  []EnvironmentDetail `json:"environments"`
	Timeline      []MetricPoint       `json:"timeline"`
	RecentEvents  []RecentEvent       `json:"recent_events"`
}

type PeriodSummary struct {
	Uptime                float64 `json:"uptime"`
	AverageResponseTimeMS float64 `json:"average_response_time_ms"`
	TotalChecks           int64   `json:"total_checks"`
	UpChecks              int64   `json:"up_checks"`
	DownChecks            int64   `json:"down_checks"`
}

type EnvironmentDetail struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	BaseURL  string          `json:"base_url"`
	Enabled  bool            `json:"enabled"`
	Status   monitor.Status  `json:"status"`
	Monitors []MonitorDetail `json:"monitors"`
}

type MonitorDetail struct {
	ID                    string         `json:"id"`
	EnvironmentID         string         `json:"environment_id"`
	Name                  string         `json:"name"`
	URL                   string         `json:"url"`
	Method                string         `json:"method"`
	Enabled               bool           `json:"enabled"`
	EffectiveEnabled      bool           `json:"effective_enabled"`
	Status                monitor.Status `json:"status"`
	LastStatusCode        int            `json:"last_status_code"`
	LastResponseTimeMS    int64          `json:"last_response_time_ms"`
	LastCheckedAt         *time.Time     `json:"last_checked_at,omitempty"`
	Uptime                float64        `json:"uptime"`
	AverageResponseTimeMS float64        `json:"average_response_time_ms"`
	TotalChecks           int64          `json:"total_checks"`
}

type MetricPoint struct {
	Timestamp             time.Time `json:"timestamp"`
	Uptime                float64   `json:"uptime"`
	AverageResponseTimeMS float64   `json:"average_response_time_ms"`
	Checks                int64     `json:"checks"`
}

type RecentEvent struct {
	ID              string         `json:"id"`
	MonitorID       string         `json:"monitor_id"`
	MonitorName     string         `json:"monitor_name"`
	EnvironmentName string         `json:"environment_name"`
	Outcome         string         `json:"outcome"`
	ComputedStatus  monitor.Status `json:"computed_status"`
	StatusCode      int            `json:"status_code"`
	ResponseTimeMS  int64          `json:"response_time_ms"`
	ErrorMessage    string         `json:"error_message,omitempty"`
	CheckedAt       time.Time      `json:"checked_at"`
}
