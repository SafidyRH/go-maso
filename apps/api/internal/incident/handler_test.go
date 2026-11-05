package incident

import (
	"testing"
	"time"
)

func TestNormalizeAndValidateAlertRule(t *testing.T) {
	t.Parallel()

	input := AlertRuleInput{Enabled: true, WebhookURL: "  https://hooks.example.com/alert  "}
	normalizeAlertRule(&input)
	if err := validateAlertRule(input, false); err != nil {
		t.Fatalf("validateAlertRule() error = %v", err)
	}
	if input.WebhookURL != "https://hooks.example.com/alert" || input.FailureThreshold != 3 || input.CooldownSeconds != 0 {
		t.Fatalf("normalized rule = %+v", input)
	}
}

func TestValidateAlertRuleRejectsEnabledRuleWithoutWebhook(t *testing.T) {
	t.Parallel()

	input := AlertRuleInput{Enabled: true}
	normalizeAlertRule(&input)
	if err := validateAlertRule(input, false); err == nil {
		t.Fatal("validateAlertRule() succeeded without webhook URL")
	}
	if err := validateAlertRule(input, true); err != nil {
		t.Fatalf("validateAlertRule() rejected an existing encrypted webhook: %v", err)
	}
}

func TestValidateMaintenance(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 3, 8, 0, 0, 0, time.UTC)
	valid := CreateMaintenanceInput{Reason: "Release", StartsAt: now, EndsAt: now.Add(time.Hour)}
	if err := validateMaintenance(valid, now); err != nil {
		t.Fatalf("validateMaintenance() error = %v", err)
	}

	tooLong := valid
	tooLong.EndsAt = now.Add(31 * 24 * time.Hour)
	if err := validateMaintenance(tooLong, now); err == nil {
		t.Fatal("validateMaintenance() accepted a 31-day window")
	}
}
