package auth

import "time"

type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	Disabled    bool       `json:"disabled"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type Session struct {
	ID            string
	User          User
	CSRFTokenHash []byte
	LastSeenAt    time.Time
	ExpiresAt     time.Time
	CreatedAt     time.Time
}

type Credentials struct {
	SessionToken string
	CSRFToken    string
	Session      Session
}

type contextKey struct{}
