package auth

import (
	"context"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeSessionStore struct {
	session Session
	err     error
}

func (s fakeSessionStore) FindSession(context.Context, string, time.Duration) (Session, error) {
	return s.session, s.err
}

func TestMiddlewareRequiresAuthenticationForAPI(t *testing.T) {
	t.Parallel()

	middleware := NewMiddleware(fakeSessionStore{err: ErrSessionNotFound}, 30*time.Minute, false)
	request := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	response := httptest.NewRecorder()
	middleware.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler was called")
	})).ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestMiddlewareAllowsPublicAssets(t *testing.T) {
	t.Parallel()

	middleware := NewMiddleware(fakeSessionStore{}, 30*time.Minute, false)
	request := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	response := httptest.NewRecorder()
	middleware.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestMiddlewareValidatesCSRFForMutation(t *testing.T) {
	t.Parallel()

	csrfToken := "csrf-token"
	csrfHash := sha256.Sum256([]byte(csrfToken))
	middleware := NewMiddleware(fakeSessionStore{session: Session{
		User:          User{ID: "user-id"},
		CSRFTokenHash: csrfHash[:],
	}}, 30*time.Minute, false)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := SessionFromContext(r.Context()); !ok {
			t.Fatal("session missing from request context")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodPost, "http://example.com/applications", nil)
	request.Host = "example.com"
	request.Header.Set("Origin", "http://example.com")
	request.Header.Set("X-CSRF-Token", csrfToken)
	request.AddCookie(&http.Cookie{Name: "go_maso_session", Value: "session-token"})
	request.AddCookie(&http.Cookie{Name: "go_maso_csrf", Value: csrfToken})
	response := httptest.NewRecorder()
	middleware.Handler(next).ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestMiddlewareRejectsMissingCSRF(t *testing.T) {
	t.Parallel()

	middleware := NewMiddleware(fakeSessionStore{session: Session{User: User{ID: "user-id"}}}, 30*time.Minute, false)
	request := httptest.NewRequest(http.MethodDelete, "/applications/id", nil)
	request.AddCookie(&http.Cookie{Name: "go_maso_session", Value: "session-token"})
	response := httptest.NewRecorder()
	middleware.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("protected handler was called")
	})).ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestLoginLimiterBlocksAfterMaximumFailures(t *testing.T) {
	t.Parallel()

	limiter := newLoginLimiter(2, time.Minute)
	now := time.Now()
	limiter.Failure("address", now)
	limiter.Failure("address", now)
	if limiter.Allow("address", now) {
		t.Fatal("limiter allowed an address at the failure limit")
	}
	if !limiter.Allow("address", now.Add(2*time.Minute)) {
		t.Fatal("limiter did not reset after its window")
	}
}
