package monitoring

import (
	"testing"

	"github.com/safidy/go-maso/apps/api/internal/monitor"
)

func TestCalculateState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		outcome          Outcome
		threshold        int
		previousFailures int
		wantStatus       monitor.Status
		wantFailures     int
	}{
		{name: "success", outcome: OutcomeUp, threshold: 3, previousFailures: 2, wantStatus: monitor.StatusUp, wantFailures: 0},
		{name: "first failure", outcome: OutcomeDown, threshold: 3, wantStatus: monitor.StatusDegraded, wantFailures: 1},
		{name: "threshold reached", outcome: OutcomeDown, threshold: 3, previousFailures: 2, wantStatus: monitor.StatusDown, wantFailures: 3},
		{name: "immediate down", outcome: OutcomeDown, threshold: 1, wantStatus: monitor.StatusDown, wantFailures: 1},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			status, failures := calculateState(test.outcome, test.threshold, test.previousFailures)
			if status != test.wantStatus || failures != test.wantFailures {
				t.Fatalf("calculateState() = (%s, %d), want (%s, %d)", status, failures, test.wantStatus, test.wantFailures)
			}
		})
	}
}
