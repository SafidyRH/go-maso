package monitoring

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/safidy/go-maso/apps/api/internal/monitor"
)

type commandType uint8

const (
	commandSync commandType = iota
	commandRemove
	commandBatch
)

type schedulerCommand struct {
	kind      commandType
	monitor   monitor.Monitor
	monitorID string
	monitors  []monitor.Monitor
	ack       chan struct{}
}

type scheduledMonitor struct {
	monitor monitor.Monitor
	nextRun time.Time
	running bool
}

type Scheduler struct {
	monitorRepository *monitor.Repository
	monitoringService *Service
	workerCount       int
	retention         time.Duration
	commands          chan schedulerCommand
	jobs              chan monitor.Monitor
	completed         chan string
	wg                sync.WaitGroup
}

func NewScheduler(
	monitorRepository *monitor.Repository,
	monitoringService *Service,
	workerCount int,
	retention time.Duration,
) *Scheduler {
	return &Scheduler{
		monitorRepository: monitorRepository,
		monitoringService: monitoringService,
		workerCount:       workerCount,
		retention:         retention,
		commands:          make(chan schedulerCommand, 64),
		jobs:              make(chan monitor.Monitor, workerCount*4),
		completed:         make(chan string, workerCount*2),
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	monitors, err := s.monitorRepository.ListEnabled(ctx)
	if err != nil {
		return err
	}

	slog.Info("starting monitoring scheduler", "monitors", len(monitors), "workers", s.workerCount)

	s.wg.Add(1)
	go s.run(ctx, monitors)

	for workerID := 1; workerID <= s.workerCount; workerID++ {
		s.wg.Add(1)
		go s.worker(ctx, workerID)
	}

	s.wg.Add(1)
	go s.retentionLoop(ctx)

	return nil
}

func (s *Scheduler) Sync(ctx context.Context, target monitor.Monitor) error {
	return s.submit(ctx, schedulerCommand{kind: commandSync, monitor: target, ack: make(chan struct{})})
}

func (s *Scheduler) Remove(ctx context.Context, monitorID string) error {
	return s.submit(ctx, schedulerCommand{kind: commandRemove, monitorID: monitorID, ack: make(chan struct{})})
}

func (s *Scheduler) RefreshEnvironment(ctx context.Context, environmentID string) error {
	monitors, err := s.monitorRepository.ListForSchedulingByEnvironment(ctx, environmentID)
	if err != nil {
		return err
	}
	return s.submit(ctx, schedulerCommand{kind: commandBatch, monitors: monitors, ack: make(chan struct{})})
}

func (s *Scheduler) RefreshApplication(ctx context.Context, applicationID string) error {
	monitors, err := s.monitorRepository.ListForSchedulingByApplication(ctx, applicationID)
	if err != nil {
		return err
	}
	return s.submit(ctx, schedulerCommand{kind: commandBatch, monitors: monitors, ack: make(chan struct{})})
}

func (s *Scheduler) Wait() {
	s.wg.Wait()
}

func (s *Scheduler) submit(ctx context.Context, command schedulerCommand) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.commands <- command:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-command.ack:
		return nil
	}
}

func (s *Scheduler) run(ctx context.Context, initial []monitor.Monitor) {
	defer s.wg.Done()

	scheduled := make(map[string]*scheduledMonitor)
	now := time.Now()
	for _, target := range initial {
		target := target
		scheduled[target.ID] = &scheduledMonitor{monitor: target, nextRun: now}
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("monitoring scheduler stopping")
			return

		case command := <-s.commands:
			s.applyCommand(scheduled, command)
			close(command.ack)

		case monitorID := <-s.completed:
			if item := scheduled[monitorID]; item != nil {
				item.running = false
			}

		case tickedAt := <-ticker.C:
			for _, item := range scheduled {
				if item.running || tickedAt.Before(item.nextRun) {
					continue
				}

				select {
				case s.jobs <- item.monitor:
					item.running = true
					item.nextRun = tickedAt.Add(time.Duration(item.monitor.CheckIntervalSeconds) * time.Second)
				default:
					// The bounded queue is full. Leave the monitor due so it is
					// retried on the next scheduler tick without spawning work.
				}
			}
		}
	}
}

func (s *Scheduler) applyCommand(scheduled map[string]*scheduledMonitor, command schedulerCommand) {
	now := time.Now()
	apply := func(target monitor.Monitor) {
		if !target.EffectiveEnabled {
			delete(scheduled, target.ID)
			return
		}

		running := false
		if existing := scheduled[target.ID]; existing != nil {
			running = existing.running
		}
		scheduled[target.ID] = &scheduledMonitor{
			monitor: target,
			nextRun: now,
			running: running,
		}
	}

	switch command.kind {
	case commandSync:
		apply(command.monitor)
	case commandRemove:
		delete(scheduled, command.monitorID)
	case commandBatch:
		for _, target := range command.monitors {
			apply(target)
		}
	}
}

func (s *Scheduler) worker(ctx context.Context, workerID int) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case target := <-s.jobs:
			result, err := s.monitoringService.CheckScheduledMonitor(ctx, target.ID)
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrMonitorDisabled) && !errors.Is(err, monitor.ErrNotFound) {
				slog.Error("automatic monitor check failed", "worker", workerID, "monitor_id", target.ID, "error", err)
			} else if err == nil {
				slog.Info(
					"automatic monitor check completed",
					"worker", workerID,
					"monitor_id", target.ID,
					"outcome", result.Outcome,
					"status", result.ComputedStatus,
					"response_time_ms", result.ResponseTimeMS,
				)
			}

			select {
			case s.completed <- target.ID:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (s *Scheduler) retentionLoop(ctx context.Context) {
	defer s.wg.Done()

	s.cleanupExpiredResults(ctx)
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cleanupExpiredResults(ctx)
		}
	}
}

func (s *Scheduler) cleanupExpiredResults(ctx context.Context) {
	deleted, err := s.monitoringService.DeleteExpiredResults(ctx, s.retention)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			slog.Error("failed to delete expired check results", "error", err)
		}
		return
	}

	if deleted > 0 {
		slog.Info("expired check results deleted", "count", deleted)
	}
}
