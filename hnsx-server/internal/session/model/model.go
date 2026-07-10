// Package model defines the Session aggregate root for the HnsX control plane.
//
// A Session is a single execution of a Domain. The control plane owns the
// session lifecycle (pending → running → completed/failed/cancelled), while the
// actual agent execution happens either in-process (legacy) or in a Python
// Runtime Worker.
package model

import (
	"errors"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

// State represents the finite states of a session lifecycle.
type State string

// Session lifecycle states.
const (
	StatePending    State = "pending"
	StateRunning    State = "running"
	StateCompleted  State = "completed"
	StateFailed     State = "failed"
	StateCancelled  State = "cancelled"
	StatePaused     State = "paused"
)

// Common errors returned by the session service and repository.
var (
	ErrSessionNotFound    = errors.New("session: not found")
	ErrInvalidSession     = errors.New("session: invalid session")
	ErrSessionNotTerminal = errors.New("session: not in a terminal state")
	ErrAlreadyTerminal    = errors.New("session: already in a terminal state")
)

// Session is the aggregate root for one execution of a Domain.
type Session struct {
	ID            string
	DomainID      string
	DomainVersion string
	Orchestration string
	State         State
	Trigger       map[string]any
	Result        *runtime.Result
	StartedAt     time.Time
	CompletedAt   *time.Time
}

// IsTerminal reports whether the session has finished and will not transition
// further without an explicit Rerun.
func (s *Session) IsTerminal() bool {
	if s == nil {
		return false
	}
	switch s.State {
	case StateCompleted, StateFailed, StateCancelled:
		return true
	default:
		return false
	}
}

// Duration returns the elapsed time since the session started. For terminal
// sessions it returns StartedAt → CompletedAt.
func (s *Session) Duration() time.Duration {
	if s == nil {
		return 0
	}
	end := time.Now().UTC()
	if s.CompletedAt != nil {
		end = *s.CompletedAt
	}
	return end.Sub(s.StartedAt)
}

// TransitionTo updates the session state. Terminal states stamp CompletedAt.
// It returns an error if the transition is invalid for Phase 1 logic; callers
// may choose to ignore transitions that are not strictly illegal.
func (s *Session) TransitionTo(state State) error {
	if s == nil {
		return ErrInvalidSession
	}
	if s.IsTerminal() {
		return ErrAlreadyTerminal
	}
	s.State = state
	if s.IsTerminal() {
		now := time.Now().UTC()
		s.CompletedAt = &now
	}
	return nil
}
