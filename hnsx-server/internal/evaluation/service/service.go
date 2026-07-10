// Package service implements the evaluation application use cases.
package service

import (
	"time"

	"github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	"github.com/hnsx-io/hnsx/server/internal/evaluation/repository"
)

// Service manages eval sets and runs.
type Service struct {
	repo repository.Repository
}

// NewService constructs a Service backed by the supplied repository.
func NewService(repo repository.Repository) *Service {
	return &Service{repo: repo}
}

// CreateSet persists a new EvalSet.
func (s *Service) CreateSet(set *model.EvalSet) error {
	if set.CreatedAt.IsZero() {
		set.CreatedAt = time.Now().UTC()
	}
	if set.UpdatedAt.IsZero() {
		set.UpdatedAt = set.CreatedAt
	}
	return s.repo.SaveSet(set)
}

// GetSet returns an EvalSet by ID.
func (s *Service) GetSet(id string) (*model.EvalSet, error) {
	return s.repo.SetByID(id)
}

// ListSets returns paginated eval sets.
func (s *Service) ListSets(limit, offset int) ([]model.EvalSet, int, error) {
	return s.repo.ListSets(limit, offset)
}

// SetsByDomain returns eval sets for a domain.
func (s *Service) SetsByDomain(domainID string) ([]model.EvalSet, error) {
	return s.repo.SetsByDomain(domainID)
}

// CreateRun starts a new EvalRun.
func (s *Service) CreateRun(run *model.EvalRun) error {
	if run.CreatedAt.IsZero() {
		run.CreatedAt = time.Now().UTC()
	}
	if run.State == "" {
		run.State = "running"
	}
	return s.repo.SaveRun(run)
}

// GetRun returns an EvalRun by ID.
func (s *Service) GetRun(id string) (*model.EvalRun, error) {
	return s.repo.RunByID(id)
}

// RunsBySet returns all runs for an EvalSet.
func (s *Service) RunsBySet(setID string) ([]model.EvalRun, error) {
	return s.repo.RunsBySet(setID)
}

// FinishRun marks a run as completed with aggregates.
func (s *Service) FinishRun(runID string, score float64, passed, total int, durationMs int64, costUSD float64) error {
	run, err := s.repo.RunByID(runID)
	if err != nil {
		return err
	}
	run.State = "completed"
	run.Score = score
	run.PassedCases = passed
	run.TotalCases = total
	run.DurationMs = durationMs
	run.TotalCostUSD = costUSD
	now := time.Now().UTC()
	run.CompletedAt = &now
	return s.repo.SaveRun(run)
}
