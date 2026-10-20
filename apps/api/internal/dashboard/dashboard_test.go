package dashboard

import (
	"net/http/httptest"
	"testing"

	"github.com/safidy/go-maso/apps/api/internal/monitor"
)

func TestParsePeriodHours(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		query   string
		want    int
		wantErr bool
	}{
		{name: "default", want: 24},
		{name: "one hour", query: "?period_hours=1", want: 1},
		{name: "one week", query: "?period_hours=168", want: 168},
		{name: "unsupported", query: "?period_hours=12", wantErr: true},
		{name: "not a number", query: "?period_hours=day", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			request := httptest.NewRequest("GET", "/dashboard/applications/id"+test.query, nil)
			got, err := parsePeriodHours(request)
			if (err != nil) != test.wantErr {
				t.Fatalf("parsePeriodHours() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("parsePeriodHours() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestWorstStatus(t *testing.T) {
	t.Parallel()

	status := monitor.StatusUnknown
	for _, candidate := range []monitor.Status{monitor.StatusUp, monitor.StatusDegraded, monitor.StatusDown, monitor.StatusUp} {
		status = worstStatus(status, candidate)
	}
	if status != monitor.StatusDown {
		t.Fatalf("worstStatus() = %s, want DOWN", status)
	}
}

func TestBucketSeconds(t *testing.T) {
	t.Parallel()

	tests := map[int]int{1: 60, 6: 300, 24: 900, 168: 3600, 720: 14400}
	for hours, want := range tests {
		if got := bucketSeconds(hours); got != want {
			t.Fatalf("bucketSeconds(%d) = %d, want %d", hours, got, want)
		}
	}
}
