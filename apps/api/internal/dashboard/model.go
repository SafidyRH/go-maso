package dashboard

import (
	"time"

	"github.com/safidy/go-maso/apps/api/internal/monitoring"
)

type ApplicationOverview struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	HealthCheckURL string `json:"health_check_url"`
	Enabled        bool   `json:"enabled"`

	Status             monitoring.Status `json:"status"`
	LastStatusCode     int               `json:"last_status_code"`
	LastResponseTimeMS int64             `json:"last_response_time_ms"`
	LastCheckedAt      *time.Time        `json:"last_checked_at,omitempty"`

	Uptime24H             float64 `json:"uptime_24h"`
	AverageResponseTimeMS float64 `json:"average_response_time_ms"`

	TotalChecks24H int64 `json:"total_checks_24h"`
	UpChecks24H    int64 `json:"up_checks_24h"`
	DownChecks24H  int64 `json:"down_checks_24h"`
}
