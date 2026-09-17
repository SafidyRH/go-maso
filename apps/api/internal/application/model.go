package application

import "time"

type Application struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	URL                  string    `json:"url"`
	HealthCheckURL       string    `json:"health_check_url"`
	CheckIntervalSeconds int       `json:"check_interval_seconds"`
	Enabled              bool      `json:"enabled"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type CreateApplicationInput struct {
	Name                 string `json:"name"`
	URL                  string `json:"url"`
	HealthCheckURL       string `json:"health_check_url"`
	CheckIntervalSeconds int    `json:"check_interval_seconds"`
}

type UpdateApplicationInput struct {
	Name                 *string `json:"name"`
	URL                  *string `json:"url"`
	HealthCheckURL       *string `json:"health_check_url"`
	CheckIntervalSeconds *int    `json:"check_interval_seconds"`
	Enabled              *bool   `json:"enabled"`
}
