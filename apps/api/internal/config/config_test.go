package config

import (
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
