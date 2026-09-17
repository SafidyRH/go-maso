package monitoring

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/safidy/go-maso/apps/api/internal/application"
)

type commandType uint8

const (
	commandSync commandType = iota
	commandRemove
)

type command struct {
	kind          commandType
	app           application.Application
	applicationID string
	ack           chan struct{}
}

type Scheduler struct {
	applicationRepository *application.Repository
	monitoringService     *Service

	commands chan command
	wg       sync.WaitGroup
}

func NewScheduler(
	applicationRepository *application.Repository,
	monitoringService *Service,
) *Scheduler {
	return &Scheduler{
		applicationRepository: applicationRepository,
		monitoringService:     monitoringService,
		commands:              make(chan command, 64),
	}
}

func (s *Scheduler) Start(ctx context.Context) error {
	applications, err := s.applicationRepository.ListEnabled(ctx)
	if err != nil {
		return err
	}

	slog.Info(
		"starting monitoring scheduler",
		"applications", len(applications),
	)

	s.wg.Add(1)

	go s.run(ctx, applications)

	return nil
}

func (s *Scheduler) Sync(
	ctx context.Context,
	app application.Application,
) error {
	cmd := command{
		kind: commandSync,
		app:  app,
		ack:  make(chan struct{}),
	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	case s.commands <- cmd:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-cmd.ack:
		return nil
	}
}

func (s *Scheduler) Remove(
	ctx context.Context,
	applicationID string,
) error {
	cmd := command{
		kind:          commandRemove,
		applicationID: applicationID,
		ack:           make(chan struct{}),
	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	case s.commands <- cmd:
	}

	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-cmd.ack:
		return nil
	}
}

func (s *Scheduler) Wait() {
	s.wg.Wait()
}

func (s *Scheduler) run(
	ctx context.Context,
	initialApplications []application.Application,
) {
	defer s.wg.Done()

	workers := make(map[string]context.CancelFunc)

	// Reconstruction des workers au démarrage.
	for _, app := range initialApplications {
		s.replaceWorker(ctx, workers, app)
	}

	defer func() {
		for _, cancel := range workers {
			cancel()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			slog.Info("monitoring scheduler stopping")
			return

		case cmd := <-s.commands:
			switch cmd.kind {

			case commandSync:
				s.replaceWorker(
					ctx,
					workers,
					cmd.app,
				)

			case commandRemove:
				s.removeWorker(
					workers,
					cmd.applicationID,
				)
			}

			close(cmd.ack)
		}
	}
}

func (s *Scheduler) replaceWorker(
	parent context.Context,
	workers map[string]context.CancelFunc,
	app application.Application,
) {
	// S'il existe déjà, on arrête l'ancien worker.
	if cancel, exists := workers[app.ID]; exists {
		cancel()
		delete(workers, app.ID)

		slog.Info(
			"monitor worker stopped for reload",
			"application", app.Name,
			"application_id", app.ID,
		)
	}

	// Une application désactivée ne doit pas avoir de worker.
	if !app.Enabled {
		slog.Info(
			"monitor disabled",
			"application", app.Name,
			"application_id", app.ID,
		)

		return
	}

	workerCtx, cancel := context.WithCancel(parent)

	workers[app.ID] = cancel

	s.wg.Add(1)

	go s.monitorApplication(
		workerCtx,
		app,
	)
}

func (s *Scheduler) removeWorker(
	workers map[string]context.CancelFunc,
	applicationID string,
) {
	cancel, exists := workers[applicationID]

	if !exists {
		return
	}

	cancel()
	delete(workers, applicationID)

	slog.Info(
		"monitor worker removed",
		"application_id", applicationID,
	)
}

func (s *Scheduler) monitorApplication(
	ctx context.Context,
	app application.Application,
) {
	defer s.wg.Done()

	interval := time.Duration(
		app.CheckIntervalSeconds,
	) * time.Second

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	slog.Info(
		"monitor started",
		"application", app.Name,
		"application_id", app.ID,
		"interval", interval,
	)

	// Check immédiat.
	s.runCheck(ctx, app)

	for {
		select {
		case <-ctx.Done():
			slog.Info(
				"monitor stopped",
				"application", app.Name,
				"application_id", app.ID,
			)

			return

		case <-ticker.C:
			s.runCheck(ctx, app)
		}
	}
}

func (s *Scheduler) runCheck(
	ctx context.Context,
	app application.Application,
) {
	checkCtx, cancel := context.WithTimeout(
		ctx,
		10*time.Second,
	)

	defer cancel()

	result, err := s.monitoringService.CheckApplication(
		checkCtx,
		app.ID,
	)

	if err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}

		slog.Error(
			"automatic health check failed",
			"application", app.Name,
			"application_id", app.ID,
			"error", err,
		)

		return
	}

	slog.Info(
		"health check completed",
		"application", app.Name,
		"status", result.Status,
		"status_code", result.StatusCode,
		"response_time_ms", result.ResponseTimeMS,
	)
}
