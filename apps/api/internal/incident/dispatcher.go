package incident

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type deliveryStore interface {
	RecoverInterruptedDeliveries(context.Context) error
	ClaimPendingDelivery(context.Context) (PendingDelivery, error)
	MarkDeliverySucceeded(context.Context, PendingDelivery, int) error
	MarkDeliveryFailed(context.Context, PendingDelivery, int, error, time.Time) error
}

type WebhookSender struct {
	client *http.Client
}

func NewWebhookSender(client *http.Client) *WebhookSender {
	return &WebhookSender{client: client}
}

func (s *WebhookSender) Send(ctx context.Context, delivery PendingDelivery) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, delivery.WebhookURL, bytes.NewReader(delivery.Payload))
	if err != nil {
		return 0, fmt.Errorf("create webhook request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "Go-Maso-Notifications/1.0")
	request.Header.Set("X-Go-Maso-Event", delivery.EventType)
	request.Header.Set("Idempotency-Key", delivery.IdempotencyKey)

	response, err := s.client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("send webhook: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return response.StatusCode, fmt.Errorf("webhook returned HTTP %d", response.StatusCode)
	}
	return response.StatusCode, nil
}

type Dispatcher struct {
	store  deliveryStore
	sender *WebhookSender
	wg     sync.WaitGroup
}

func NewDispatcher(store deliveryStore, sender *WebhookSender) *Dispatcher {
	return &Dispatcher{store: store, sender: sender}
}

func (d *Dispatcher) Start(ctx context.Context) {
	d.wg.Add(1)
	go d.run(ctx)
}

func (d *Dispatcher) Wait() {
	d.wg.Wait()
}

func (d *Dispatcher) run(ctx context.Context) {
	defer d.wg.Done()
	if err := d.store.RecoverInterruptedDeliveries(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("failed to recover interrupted notifications", "error", err)
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		if err := d.deliverOne(ctx); err != nil && !errors.Is(err, ErrNoPendingDelivery) && !errors.Is(err, context.Canceled) {
			slog.Error("notification delivery cycle failed", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (d *Dispatcher) deliverOne(ctx context.Context) error {
	delivery, err := d.store.ClaimPendingDelivery(ctx)
	if err != nil {
		return err
	}

	deliveryCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	status, sendErr := d.sender.Send(deliveryCtx, delivery)
	if sendErr == nil {
		return d.store.MarkDeliverySucceeded(ctx, delivery, status)
	}

	nextAttempt := time.Now().UTC().Add(retryDelay(delivery.Attempts))
	return d.store.MarkDeliveryFailed(ctx, delivery, status, sendErr, nextAttempt)
}

func retryDelay(attempt int) time.Duration {
	delays := []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 4 * time.Hour}
	if attempt <= 0 {
		return delays[0]
	}
	if attempt > len(delays) {
		return delays[len(delays)-1]
	}
	return delays[attempt-1]
}
