package monitoring

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/safidy/go-maso/apps/api/internal/monitor"
)

var ErrMonitorDisabled = errors.New("monitor is disabled")

type Service struct {
	monitorRepository    *monitor.Repository
	monitoringRepository *Repository
	checker              *Checker
}

func NewService(
	monitorRepository *monitor.Repository,
	monitoringRepository *Repository,
	checker *Checker,
) *Service {
	return &Service{
		monitorRepository:    monitorRepository,
		monitoringRepository: monitoringRepository,
		checker:              checker,
	}
}

func (s *Service) CheckMonitor(ctx context.Context, monitorID string) (CheckResult, error) {
	return s.check(ctx, monitorID, false)
}

func (s *Service) CheckScheduledMonitor(ctx context.Context, monitorID string) (CheckResult, error) {
	return s.check(ctx, monitorID, true)
}

func (s *Service) check(ctx context.Context, monitorID string, requireEnabled bool) (CheckResult, error) {
	target, err := s.monitorRepository.GetByID(ctx, monitorID)
	if err != nil {
		return CheckResult{}, err
	}
	if requireEnabled && !target.EffectiveEnabled {
		return CheckResult{}, ErrMonitorDisabled
	}

	check := s.checker.Check(ctx, target)
	result, err := s.monitoringRepository.Record(ctx, check)
	if err != nil {
		return CheckResult{}, fmt.Errorf("save check result: %w", err)
	}

	return result, nil
}

func (s *Service) GetHistory(ctx context.Context, monitorID string, limit int) ([]CheckResult, error) {
	if _, err := s.monitorRepository.GetByID(ctx, monitorID); err != nil {
		return nil, err
	}
	return s.monitoringRepository.ListByMonitor(ctx, monitorID, limit)
}

func (s *Service) GetSummary(ctx context.Context, monitorID string) (Summary, error) {
	if _, err := s.monitorRepository.GetByID(ctx, monitorID); err != nil {
		return Summary{}, err
	}
	return s.monitoringRepository.GetSummary(ctx, monitorID)
}

func (s *Service) DeleteExpiredResults(ctx context.Context, retention time.Duration) (int64, error) {
	return s.monitoringRepository.DeleteOlderThan(ctx, time.Now().UTC().Add(-retention))
}
