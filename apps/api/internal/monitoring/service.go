package monitoring

import (
	"context"
	"fmt"

	"github.com/safidy/go-maso/apps/api/internal/application"
)

type Service struct {
	applicationRepository *application.Repository
	monitoringRepository  *Repository
	checker               *Checker
}

func NewService(
	applicationRepository *application.Repository,
	monitoringRepository *Repository,
	checker *Checker,
) *Service {
	return &Service{
		applicationRepository: applicationRepository,
		monitoringRepository:  monitoringRepository,
		checker:               checker,
	}
}

func (s *Service) CheckApplication(
	ctx context.Context,
	applicationID string,
) (HealthCheck, error) {
	app, err := s.applicationRepository.GetByID(
		ctx,
		applicationID,
	)

	if err != nil {
		return HealthCheck{}, err
	}

	check := s.checker.Check(ctx, app)

	result, err := s.monitoringRepository.Create(
		ctx,
		check,
	)

	if err != nil {
		return HealthCheck{}, fmt.Errorf(
			"save health check: %w",
			err,
		)
	}

	return result, nil
}

func (s *Service) GetHistory(
	ctx context.Context,
	applicationID string,
	limit int,
) ([]HealthCheck, error) {
	_, err := s.applicationRepository.GetByID(
		ctx,
		applicationID,
	)

	if err != nil {
		return nil, err
	}

	return s.monitoringRepository.ListByApplication(
		ctx,
		applicationID,
		limit,
	)
}

func (s *Service) GetSummary(
	ctx context.Context,
	applicationID string,
) (Summary, error) {
	_, err := s.applicationRepository.GetByID(
		ctx,
		applicationID,
	)

	if err != nil {
		return Summary{}, err
	}

	return s.monitoringRepository.GetSummary(
		ctx,
		applicationID,
	)
}
