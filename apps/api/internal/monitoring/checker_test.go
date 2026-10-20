package monitoring

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/safidy/go-maso/apps/api/internal/monitor"
)

func TestCheckerUsesConfiguredMethodAndStatusRange(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %s, want HEAD", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	checker := &Checker{client: server.Client()}
	result := checker.Check(context.Background(), monitor.Monitor{
		ID:                "monitor-1",
		URL:               server.URL,
		Method:            http.MethodHead,
		TimeoutMS:         1000,
		ExpectedStatusMin: 200,
		ExpectedStatusMax: 299,
	})

	if result.Outcome != OutcomeUp || result.StatusCode != http.StatusNoContent {
		t.Fatalf("result = %#v, want UP with status 204", result)
	}
}

func TestCheckerRejectsInvalidTLSCertificate(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := &Checker{client: http.DefaultClient}
	result := checker.Check(context.Background(), monitor.Monitor{
		ID:                "monitor-1",
		URL:               server.URL,
		Method:            http.MethodGet,
		TimeoutMS:         1000,
		ExpectedStatusMin: 200,
		ExpectedStatusMax: 299,
	})

	if result.Outcome != OutcomeDown || result.ErrorMessage == "" {
		t.Fatalf("result = %#v, want TLS failure", result)
	}
}

type failingResolver struct{}

func (failingResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return nil, errors.New("host not found")
}

func TestCheckerReportsDNSFailureBeforeRequest(t *testing.T) {
	t.Parallel()

	policy := &NetworkPolicy{resolver: failingResolver{}}
	checker := NewChecker(policy)
	result := checker.Check(context.Background(), monitor.Monitor{
		ID:                "monitor-1",
		URL:               "https://unresolvable.test/health",
		Method:            http.MethodGet,
		TimeoutMS:         1000,
		ExpectedStatusMin: 200,
		ExpectedStatusMax: 299,
	})

	if result.Outcome != OutcomeDown || !strings.Contains(result.ErrorMessage, "resolve") {
		t.Fatalf("result = %#v, want DNS failure", result)
	}
}

func TestCheckerRedirectPolicyRejectsInternalTarget(t *testing.T) {
	t.Parallel()

	policy := &NetworkPolicy{resolver: fakeResolver{}}
	checker := NewChecker(policy)
	request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/private", nil)

	if err := checker.client.CheckRedirect(request, nil); err == nil {
		t.Fatal("CheckRedirect succeeded, want internal target rejection")
	}
}

func TestCheckerReportsUnexpectedStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	checker := &Checker{client: server.Client()}
	result := checker.Check(context.Background(), monitor.Monitor{
		ID:                "monitor-1",
		URL:               server.URL,
		Method:            http.MethodGet,
		TimeoutMS:         1000,
		ExpectedStatusMin: 200,
		ExpectedStatusMax: 299,
	})

	if result.Outcome != OutcomeDown || result.StatusCode != http.StatusInternalServerError || result.ErrorMessage == "" {
		t.Fatalf("result = %#v, want DOWN with status error", result)
	}
}

func TestCheckerHonorsTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := &Checker{client: server.Client()}
	result := checker.Check(context.Background(), monitor.Monitor{
		ID:                "monitor-1",
		URL:               server.URL,
		Method:            http.MethodGet,
		TimeoutMS:         10,
		ExpectedStatusMin: 200,
		ExpectedStatusMax: 299,
	})

	if result.Outcome != OutcomeDown || result.ErrorMessage == "" {
		t.Fatalf("result = %#v, want timeout failure", result)
	}
}
