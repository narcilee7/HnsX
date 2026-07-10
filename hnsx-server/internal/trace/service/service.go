// Package service implements the trace application use cases.
package service

import (
	"context"

	"github.com/hnsx-io/hnsx/server/internal/trace/model"
	"github.com/hnsx-io/hnsx/server/internal/trace/repository"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

// Service records and queries traces.
type Service struct {
	repo repository.Repository
}

// NewService constructs a Service backed by the supplied repository.
func NewService(repo repository.Repository) *Service {
	return &Service{repo: repo}
}

// Record persists a runtime observation.
func (s *Service) Record(ctx context.Context, obs runtime.Observation) error {
	rec := model.FromRuntime(obs)
	return s.repo.Save(&rec)
}

// Aggregate returns rolled-up cost/token/invocation counts for the given
// session IDs.
func (s *Service) Aggregate(sessionIDs []string) (model.Aggregate, error) {
	return s.repo.Aggregate(sessionIDs)
}

// BySession returns observations for a session in chronological order.
func (s *Service) BySession(sessionID string) ([]model.ObservationRecord, error) {
	return s.repo.BySession(sessionID)
}

// ByTrace returns observations for a trace in chronological order.
func (s *Service) ByTrace(traceID string) ([]model.ObservationRecord, error) {
	return s.repo.ByTrace(traceID)
}
