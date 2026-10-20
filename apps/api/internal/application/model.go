package application

import "time"

type Application struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Owner      string     `json:"owner"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type CreateApplicationInput struct {
	Name  string `json:"name"`
	Owner string `json:"owner"`
}

type UpdateApplicationInput struct {
	Name  *string `json:"name"`
	Owner *string `json:"owner"`
}

type Page struct {
	Items  []Application `json:"items"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
}
