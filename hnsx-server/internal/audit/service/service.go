// Package service implements the audit application use cases.
//
// It records policy decisions, tool invocations, and session lifecycle events
// into an immutable audit trail.
package service

import (
	"context"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/audit/model"
	"github.com/hnsx-io/hnsx/server/internal/audit/repository"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

// Service records and queries audit entries.
type Service struct {
	repo repository.Repository
}

// NewService constructs a Service backed by the supplied repository.
func NewService(repo repository.Repository) *Service {
	return &Service{repo: repo}
}

// Record persists a single audit entry.
func (s *Service) Record(ctx context.Context, e *model.Entry) error {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	return s.repo.Save(e)
}

// RecordObservation derives an audit entry from a runtime observation.
func (s *Service) RecordObservation(ctx context.Context, obs runtime.Observation, decision, reason string) error {
	return s.Record(ctx, &model.Entry{
		SessionID: obs.SessionID,
		DomainID:  obs.DomainID,
		Action:    obs.Kind,
		Actor:     firstNonEmpty(obs.AgentID, "system"),
		ActorType: model.ActorTypeAgent,
		Decision:  decision,
		Reason:    reason,
		Details:   obs.Payload,
		Timestamp: obs.Timestamp,
	})
}

// RecordPolicyDecision records an explicit policy allow/deny decision.
func (s *Service) RecordPolicyDecision(ctx context.Context, sessionID, domainID, action, resource, decision, reason string, details map[string]any) error {
	return s.Record(ctx, &model.Entry{
		SessionID:    sessionID,
		DomainID:     domainID,
		Action:       action,
		Actor:        "policy_engine",
		ActorType:    model.ActorTypeSystem,
		Resource:     resource,
		ResourceType: "policy",
		Decision:     decision,
		Reason:       reason,
		Details:      details,
	})
}

// BySession returns audit entries for a session, newest first.
func (s *Service) BySession(sessionID string) ([]model.Entry, error) {
	return s.repo.BySession(sessionID)
}

// ByDomain returns audit entries for a domain, newest first.
func (s *Service) ByDomain(domainID string) ([]model.Entry, error) {
	return s.repo.ByDomain(domainID)
}

// List returns paginated audit entries, newest first.
func (s *Service) List(limit, offset int) ([]model.Entry, int, error) {
	return s.repo.List(limit, offset)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
