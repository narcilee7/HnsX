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

// AggregateBySession returns a per-session rollup keyed by session ID for the
// given session IDs. Sessions with no observations are omitted.
func (s *Service) AggregateBySession(sessionIDs []string) (map[string]model.Aggregate, error) {
	return s.repo.AggregateBySession(sessionIDs)
}

// BySession returns observations for a session in chronological order.
func (s *Service) BySession(sessionID string) ([]model.ObservationRecord, error) {
	return s.repo.BySession(sessionID)
}

// ByTrace returns observations for a trace in chronological order.
func (s *Service) ByTrace(traceID string) ([]model.ObservationRecord, error) {
	return s.repo.ByTrace(traceID)
}

// ListSummaries returns a paginated list of TraceSummary records matching the
// filter, plus the total count of matching trace_ids. It is the single
// read path for GET /api/v1/traces.
func (s *Service) ListSummaries(filter model.TraceListFilter) (model.TraceSummaryWithCount, error) {
	return s.repo.ListSummaries(filter)
}

// Detail returns the full trace: per-trace rollup + chronological
// observations. ErrTraceNotFound surfaces from the repository and is
// mapped to a 404 by the HTTP handler.
func (s *Service) Detail(traceID string) (*model.TraceDetail, error) {
	return s.repo.Detail(traceID)
}
