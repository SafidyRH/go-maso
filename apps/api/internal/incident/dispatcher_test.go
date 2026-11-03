package incident

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeDeliveryStore struct {
	delivery  PendingDelivery
	claimErr  error
	succeeded bool
	failed    bool
	status    int
}

func (s *fakeDeliveryStore) RecoverInterruptedDeliveries(context.Context) error { return nil }
func (s *fakeDeliveryStore) ClaimPendingDelivery(context.Context) (PendingDelivery, error) {
	return s.delivery, s.claimErr
}
func (s *fakeDeliveryStore) MarkDeliverySucceeded(_ context.Context, _ PendingDelivery, status int) error {
	s.succeeded = true
	s.status = status
	return nil
}
func (s *fakeDeliveryStore) MarkDeliveryFailed(_ context.Context, _ PendingDelivery, status int, _ error, _ time.Time) error {
	s.failed = true
	s.status = status
	return nil
}

func TestDispatcherDeliversWebhookWithIdempotencyKey(t *testing.T) {
	t.Parallel()

	var gotEvent, gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotEvent = r.Header.Get("X-Go-Maso-Event")
		gotKey = r.Header.Get("Idempotency-Key")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	store := &fakeDeliveryStore{delivery: PendingDelivery{
		ID:             "delivery-id",
		EventType:      "INCIDENT_OPENED",
		WebhookURL:     server.URL,
		Payload:        []byte(`{"event":"INCIDENT_OPENED"}`),
		IdempotencyKey: "incident-id:INCIDENT_OPENED",
	}}
	dispatcher := NewDispatcher(store, NewWebhookSender(server.Client()))
	if err := dispatcher.deliverOne(context.Background()); err != nil {
		t.Fatalf("deliverOne() error = %v", err)
	}
	if !store.succeeded || store.failed || store.status != http.StatusNoContent {
		t.Fatalf("delivery state succeeded=%v failed=%v status=%d", store.succeeded, store.failed, store.status)
	}
	if gotEvent != "INCIDENT_OPENED" || gotKey != "incident-id:INCIDENT_OPENED" {
		t.Fatalf("webhook headers event=%q key=%q", gotEvent, gotKey)
	}
}

func TestDispatcherSchedulesRetryAfterWebhookFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	store := &fakeDeliveryStore{delivery: PendingDelivery{
		WebhookURL:  server.URL,
		Payload:     []byte(`{}`),
		Attempts:    1,
		MaxAttempts: 5,
	}}
	dispatcher := NewDispatcher(store, NewWebhookSender(server.Client()))
	if err := dispatcher.deliverOne(context.Background()); err != nil {
		t.Fatalf("deliverOne() error = %v", err)
	}
	if !store.failed || store.succeeded || store.status != http.StatusServiceUnavailable {
		t.Fatalf("delivery state succeeded=%v failed=%v status=%d", store.succeeded, store.failed, store.status)
	}
}

func TestDispatcherReturnsNoPendingDelivery(t *testing.T) {
	t.Parallel()

	store := &fakeDeliveryStore{claimErr: ErrNoPendingDelivery}
	dispatcher := NewDispatcher(store, NewWebhookSender(http.DefaultClient))
	if err := dispatcher.deliverOne(context.Background()); !errors.Is(err, ErrNoPendingDelivery) {
		t.Fatalf("deliverOne() error = %v, want ErrNoPendingDelivery", err)
	}
}

func TestRetryDelayIsBounded(t *testing.T) {
	t.Parallel()

	if got := retryDelay(1); got != time.Minute {
		t.Fatalf("retryDelay(1) = %v, want 1m", got)
	}
	if got := retryDelay(100); got != 4*time.Hour {
		t.Fatalf("retryDelay(100) = %v, want 4h", got)
	}
}
