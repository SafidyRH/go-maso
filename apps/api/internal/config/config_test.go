package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := load(func(name string) string {
		if name == "DATABASE_URL" {
			return "postgres://example"
		}

		return ""
	})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.HTTPAddress != ":8080" {
		t.Fatalf("HTTPAddress = %q, want %q", cfg.HTTPAddress, ":8080")
	}

	if cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("ShutdownTimeout = %s, want 10s", cfg.ShutdownTimeout)
	}
	if cfg.MonitorWorkers != 10 {
		t.Fatalf("MonitorWorkers = %d, want 10", cfg.MonitorWorkers)
	}
	if cfg.RetentionDays != 30 {
		t.Fatalf("RetentionDays = %d, want 30", cfg.RetentionDays)
	}
	if cfg.SessionIdle != 30*time.Minute || cfg.SessionAbsolute != 12*time.Hour {
		t.Fatalf("session timeouts = (%s, %s), want (30m, 12h)", cfg.SessionIdle, cfg.SessionAbsolute)
	}
	if cfg.SessionSecure || cfg.TrustProxy {
		t.Fatal("secure cookie and proxy trust must default to false for local development")
	}
}

func TestLoadReadsSensitiveValuesFromFiles(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"DATABASE_URL_FILE":           "/run/secrets/database_url",
		"ADMIN_PASSWORD_FILE":         "/run/secrets/admin_password",
		"SECRETS_ENCRYPTION_KEY_FILE": "/run/secrets/encryption_key",
	}
	files := map[string]string{
		"/run/secrets/database_url":   "postgres://secret\n",
		"/run/secrets/admin_password": "admin secret\n",
		"/run/secrets/encryption_key": "base64 key\n",
	}
	cfg, err := loadWith(
		func(name string) string { return values[name] },
		func(path string) ([]byte, error) {
			value, ok := files[path]
			if !ok {
				return nil, errors.New("missing test secret")
			}
			return []byte(value), nil
		},
	)
	if err != nil {
		t.Fatalf("loadWith() error = %v", err)
	}
	if cfg.DatabaseURL != "postgres://secret" || cfg.AdminPassword != "admin secret" || cfg.SecretsEncryptionKey != "base64 key" {
		t.Fatalf("file-backed secrets were not loaded: %+v", cfg)
	}
}

func TestLoadRejectsSecretValueAndFileTogether(t *testing.T) {
	t.Parallel()

	_, err := loadWith(func(name string) string {
		switch name {
		case "DATABASE_URL":
			return "postgres://value"
		case "DATABASE_URL_FILE":
			return "/run/secrets/database_url"
		default:
			return ""
		}
	}, func(string) ([]byte, error) { return nil, nil })
	if err == nil || !strings.Contains(err.Error(), "cannot both be set") {
		t.Fatalf("loadWith() error = %v, want conflicting secret sources", err)
	}
}

func TestLoadRejectsSessionAbsoluteTimeoutBeforeIdleTimeout(t *testing.T) {
	t.Parallel()

	_, err := load(func(name string) string {
		switch name {
		case "DATABASE_URL":
			return "postgres://example"
		case "SESSION_IDLE_TIMEOUT":
			return "2h"
		case "SESSION_ABSOLUTE_TIMEOUT":
			return "1h"
		default:
			return ""
		}
	})
	if err == nil || !strings.Contains(err.Error(), "SESSION_ABSOLUTE_TIMEOUT") {
		t.Fatalf("load error = %v, want session timeout ordering error", err)
	}
}

func TestLoadRejectsMissingDatabaseURL(t *testing.T) {
	t.Parallel()

	_, err := load(func(string) string { return "" })
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("load error = %v, want DATABASE_URL error", err)
	}
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	t.Parallel()

	_, err := load(func(name string) string {
		switch name {
		case "DATABASE_URL":
			return "postgres://example"
		case "SHUTDOWN_TIMEOUT":
			return "soon"
		default:
			return ""
		}
	})
	if err == nil || !strings.Contains(err.Error(), "SHUTDOWN_TIMEOUT") {
		t.Fatalf("load error = %v, want SHUTDOWN_TIMEOUT error", err)
	}
}
