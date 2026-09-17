package monitoring

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/safidy/go-maso/apps/api/internal/application"
)

type Scheduler struct {
	applicationRepository *application.Repository
	monitoringService     *Service

	wg sync.WaitGroup
}

func NewScheduler(
	applicationRepository *application.Repository,
	monitoringService *Service,
) *Scheduler {
	return &Scheduler{
		applicationRepository: applicationRepository,
		monitoringService:     monitoringService,
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

	for _, app := range applications {
		s.wg.Add(1)

		go s.monitorApplication(ctx, app)
	}

	return nil
}

func (s *Scheduler) Wait() {
	s.wg.Wait()
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
		"interval", interval,
	)

	// Premier check immédiatement.
	s.runCheck(ctx, app)

	for {
		select {
		case <-ctx.Done():
			slog.Info(
				"monitor stopped",
				"application", app.Name,
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
