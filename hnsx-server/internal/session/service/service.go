// Package service implements the session application use cases.
//
// It owns session creation, state transitions, cancellation, and rerun. The
// actual execution is delegated to the infrastructure layer (in-process
// executor or worker queue), but the lifecycle metadata lives here.
package service

import (
	"time"

	"github.com/hnsx-io/hnsx/server/internal/session/model"
	"github.com/hnsx-io/hnsx/server/internal/session/repository"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

// Service implements the session application use cases.
type Service struct {
	repo repository.Repository
}

// NewService constructs a Service backed by the supplied repository.
func NewService(repo repository.Repository) *Service {
	return &Service{repo: repo}
}

// CreateParams contains the inputs needed to create a new session.
type CreateParams struct {
	SessionID     string
	DomainID      string
	DomainVersion string
	Orchestration string
	Trigger       map[string]any
}

// Create registers a new pending session.
func (s *Service) Create(p CreateParams) (*model.Session, error) {
	if p.SessionID == "" || p.DomainID == "" {
		return nil, model.ErrInvalidSession
	}
	sess := &model.Session{
		ID:            p.SessionID,
		DomainID:      p.DomainID,
		DomainVersion: p.DomainVersion,
		Orchestration: p.Orchestration,
		State:         model.StatePending,
		Trigger:       p.Trigger,
		StartedAt:     time.Now().UTC(),
	}
	if err := s.repo.Save(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// Get returns a session by ID.
func (s *Service) Get(id string) (*model.Session, error) {
	return s.repo.ByID(id)
}

// List returns all sessions.
func (s *Service) List() ([]*model.Session, error) {
	return s.repo.All()
}

// ListByDomain returns all sessions for a domain.
func (s *Service) ListByDomain(domainID string) ([]*model.Session, error) {
	return s.repo.ByDomain(domainID)
}

// MarkRunning transitions a session from pending to running.
func (s *Service) MarkRunning(id string) (*model.Session, error) {
	sess, err := s.repo.ByID(id)
	if err != nil {
		return nil, err
	}
	if sess.State != model.StatePending {
		return nil, model.ErrInvalidSession
	}
	sess.State = model.StateRunning
	if err := s.repo.Save(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// MarkCompleted stores the final result and transitions to completed.
func (s *Service) MarkCompleted(id string, result *runtime.Result) (*model.Session, error) {
	sess, err := s.repo.ByID(id)
	if err != nil {
		return nil, err
	}
	sess.Result = result
	if err := sess.TransitionTo(model.StateCompleted); err != nil {
		return nil, err
	}
	if err := s.repo.Save(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// MarkFailed transitions the session to failed.
func (s *Service) MarkFailed(id string) (*model.Session, error) {
	sess, err := s.repo.ByID(id)
	if err != nil {
		return nil, err
	}
	if err := sess.TransitionTo(model.StateFailed); err != nil {
		return nil, err
	}
	if err := s.repo.Save(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// Cancel transitions a non-terminal session to cancelled.
func (s *Service) Cancel(id string) (*model.Session, error) {
	sess, err := s.repo.ByID(id)
	if err != nil {
		return nil, err
	}
	if sess.IsTerminal() {
		return nil, model.ErrAlreadyTerminal
	}
	if err := sess.TransitionTo(model.StateCancelled); err != nil {
		return nil, err
	}
	if err := s.repo.Save(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// UpdateState is a generic state update used by infrastructure adapters that
// receive worker status reports. It refuses to move a terminal session
// backwards.
func (s *Service) UpdateState(id string, state model.State) (*model.Session, error) {
	sess, err := s.repo.ByID(id)
	if err != nil {
		return nil, err
	}
	if sess.IsTerminal() {
		return nil, model.ErrAlreadyTerminal
	}
	sess.State = state
	if sess.IsTerminal() {
		now := time.Now().UTC()
		sess.CompletedAt = &now
	}
	if err := s.repo.Save(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// Rerun creates a new session reusing the trigger of an existing session.
func (s *Service) Rerun(existing *model.Session) (*model.Session, error) {
	if existing == nil {
		return nil, model.ErrInvalidSession
	}
	return s.Create(CreateParams{
		SessionID:     runtime.NewSessionID(existing.DomainID),
		DomainID:      existing.DomainID,
		DomainVersion: existing.DomainVersion,
		Orchestration: existing.Orchestration,
		Trigger:       existing.Trigger,
	})
}
