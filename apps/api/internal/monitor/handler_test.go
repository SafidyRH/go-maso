package monitor

import (
	"net/http"
	"testing"
)

func TestNormalizeCreateDefaults(t *testing.T) {
	t.Parallel()

	input := CreateMonitorInput{Name: " API ", URL: " https://example.com/health "}
	normalizeCreate(&input)

	if input.Name != "API" || input.URL != "https://example.com/health" {
		t.Fatalf("normalized identity = (%q, %q)", input.Name, input.URL)
	}
	if input.Method != http.MethodGet || input.CheckIntervalSeconds != 30 || input.TimeoutMS != 5000 {
		t.Fatalf("normalized defaults = %#v", input)
	}
	if input.ExpectedStatusMin != 200 || input.ExpectedStatusMax != 299 || input.FailureThreshold != 3 {
		t.Fatalf("normalized monitoring defaults = %#v", input)
	}
}

func TestValidateInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		method    string
		interval  int
		timeout   int
		statusMin int
		statusMax int
		threshold int
		wantErr   bool
	}{
		{name: "valid", method: "GET", interval: 30, timeout: 5000, statusMin: 200, statusMax: 299, threshold: 3},
		{name: "method", method: "POST", interval: 30, timeout: 5000, statusMin: 200, statusMax: 299, threshold: 3, wantErr: true},
		{name: "interval", method: "GET", interval: 4, timeout: 5000, statusMin: 200, statusMax: 299, threshold: 3, wantErr: true},
		{name: "timeout", method: "GET", interval: 30, timeout: 99, statusMin: 200, statusMax: 299, threshold: 3, wantErr: true},
		{name: "status range", method: "GET", interval: 30, timeout: 5000, statusMin: 500, statusMax: 200, threshold: 3, wantErr: true},
		{name: "threshold", method: "GET", interval: 30, timeout: 5000, statusMin: 200, statusMax: 299, threshold: 0, wantErr: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateInput("API", "https://example.com/health", test.method, test.interval, test.timeout, test.statusMin, test.statusMax, test.threshold)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateInput() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
