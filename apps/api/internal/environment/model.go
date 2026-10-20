package environment

import "time"

type Environment struct {
	ID            string     `json:"id"`
	ApplicationID string     `json:"application_id"`
	Name          string     `json:"name"`
	BaseURL       string     `json:"base_url"`
	Enabled       bool       `json:"enabled"`
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type CreateEnvironmentInput struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Enabled *bool  `json:"enabled"`
}

type UpdateEnvironmentInput struct {
	Name    *string `json:"name"`
	BaseURL *string `json:"base_url"`
	Enabled *bool   `json:"enabled"`
}

type Page struct {
	Items  []Environment `json:"items"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}
