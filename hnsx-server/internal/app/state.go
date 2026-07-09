// Package app holds the in-process application state shared by the HTTP API,
// the gRPC control plane, and the CLI.
//
// It deliberately does NOT contain protocol concerns (chi, gRPC handlers, SSE
// serialization). Those live in pkg/api and pkg/controlplane. The app layer
// owns the domain/session/broadcaster registries and delegates persistence
// and execution to the domain services.
package app

import (
	"context"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/session/broadcaster"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// RegisteredDomain is the runtime view of a loaded DomainSpec.
type RegisteredDomain struct {
	ID          string
	Version     string
	Description string
	Spec        *spec.DomainSpec
	Harness     any
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// RegisteredSession is the runtime metadata for one Session run.
type RegisteredSession struct {
	ID            string
	DomainID      string
	DomainVersion string
	Orchestration string
	State         string
	Trigger       map[string]any
	Result        *runtime.Result
	StartedAt     time.Time
	CompletedAt   *time.Time
}

// State holds the in-process registries. It is the single source of truth for
// the current execution window; persistence happens via the domain services.
type State struct {
	mu           sync.RWMutex
	domains      map[string]*RegisteredDomain
	sessions     map[string]*RegisteredSession
	broadcasters map[string]*broadcaster.Broadcaster
}

// NewState constructs an empty application state.
func NewState() *State {
	return &State{
		domains:      map[string]*RegisteredDomain{},
		sessions:     map[string]*RegisteredSession{},
		broadcasters: map[string]*broadcaster.Broadcaster{},
	}
}

// RegisterDomain inserts or replaces a domain.
func (s *State) RegisterDomain(d *RegisteredDomain) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.domains[d.ID] = d
}

// LookupDomain returns a domain by ID.
func (s *State) LookupDomain(id string) (*RegisteredDomain, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.domains[id]
	return d, ok
}

// ListDomains returns every registered domain.
func (s *State) ListDomains() []*RegisteredDomain {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*RegisteredDomain, 0, len(s.domains))
	for _, d := range s.domains {
		out = append(out, d)
	}
	return out
}

// DeleteDomain removes a domain by ID.
func (s *State) DeleteDomain(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.domains, id)
}

// RegisterSession inserts or replaces a session.
func (s *State) RegisterSession(sess *RegisteredSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
}

// LookupSession returns a session by ID.
func (s *State) LookupSession(id string) (*RegisteredSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.sessions[id]
	return sess, ok
}

// ListSessions returns every registered session.
func (s *State) ListSessions() []*RegisteredSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*RegisteredSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	return out
}

// UpdateSessionState updates the in-memory session state. Terminal states
// stamp CompletedAt.
func (s *State) UpdateSessionState(sessionID, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	sess.State = state
	if state == "completed" || state == "failed" || state == "cancelled" || state == "canceled" {
		now := time.Now().UTC()
		sess.CompletedAt = &now
	}
}

// SetSessionResult stores the final result on a session.
func (s *State) SetSessionResult(sessionID string, result *runtime.Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[sessionID]
	if !ok {
		return
	}
	sess.Result = result
}

// AttachBroadcaster returns the existing broadcaster for a session or creates
// a new one.
func (s *State) AttachBroadcaster(sessionID string) *broadcaster.Broadcaster {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bc, ok := s.broadcasters[sessionID]; ok {
		return bc
	}
	bc := broadcaster.NewBroadcaster()
	s.broadcasters[sessionID] = bc
	return bc
}

// DetachBroadcaster closes and removes a session's broadcaster.
func (s *State) DetachBroadcaster(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if bc, ok := s.broadcasters[sessionID]; ok {
		bc.Close()
		delete(s.broadcasters, sessionID)
	}
}

// PublishObservation forwards an observation into the named session's
// broadcaster. Returns false if the session has no broadcaster.
func (s *State) PublishObservation(sessionID string, obs runtime.Observation) bool {
	bc := s.AttachBroadcaster(sessionID)
	ctx := context.Background()
	if err := bc.Publish(ctx, obs); err != nil {
		return false
	}
	return true
}

// Broadcaster returns the broadcaster for a session if one exists.
func (s *State) Broadcaster(sessionID string) (*broadcaster.Broadcaster, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	bc, ok := s.broadcasters[sessionID]
	return bc, ok
}
