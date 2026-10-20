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
