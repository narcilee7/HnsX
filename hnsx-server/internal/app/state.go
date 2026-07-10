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
	"github.com/hnsx-io/hnsx/server/internal/tenant"
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
	domains      map[tenant.ID]map[string]*RegisteredDomain
	sessions     map[tenant.ID]map[string]*RegisteredSession
	broadcasters map[string]*broadcaster.Broadcaster
}

// NewState constructs an empty application state.
func NewState() *State {
	return &State{
		domains:      map[tenant.ID]map[string]*RegisteredDomain{},
		sessions:     map[tenant.ID]map[string]*RegisteredSession{},
		broadcasters: map[string]*broadcaster.Broadcaster{},
	}
}

func (s *State) tenantDomains(t tenant.ID) map[string]*RegisteredDomain {
	if s.domains[t] == nil {
		s.domains[t] = map[string]*RegisteredDomain{}
	}
	return s.domains[t]
}

func (s *State) tenantSessions(t tenant.ID) map[string]*RegisteredSession {
	if s.sessions[t] == nil {
		s.sessions[t] = map[string]*RegisteredSession{}
	}
	return s.sessions[t]
}

// RegisterDomain inserts or replaces a domain for the given tenant.
func (s *State) RegisterDomain(t tenant.ID, d *RegisteredDomain) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenantDomains(t)[d.ID] = d
}

// LookupDomain returns a domain by ID within a tenant.
func (s *State) LookupDomain(t tenant.ID, id string) (*RegisteredDomain, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.tenantDomains(t)[id]
	return d, ok
}

// ListDomains returns every registered domain for the given tenant.
func (s *State) ListDomains(t tenant.ID) []*RegisteredDomain {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.tenantDomains(t)
	out := make([]*RegisteredDomain, 0, len(m))
	for _, d := range m {
		out = append(out, d)
	}
	return out
}

// DeleteDomain removes a domain by ID within a tenant.
func (s *State) DeleteDomain(t tenant.ID, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.tenantDomains(t), id)
}

// RegisterSession inserts or replaces a session for the given tenant.
func (s *State) RegisterSession(t tenant.ID, sess *RegisteredSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tenantSessions(t)[sess.ID] = sess
}

// LookupSession returns a session by ID within a tenant.
func (s *State) LookupSession(t tenant.ID, id string) (*RegisteredSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.tenantSessions(t)[id]
	return sess, ok
}

// ListSessions returns every registered session for the given tenant.
func (s *State) ListSessions(t tenant.ID) []*RegisteredSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m := s.tenantSessions(t)
	out := make([]*RegisteredSession, 0, len(m))
	for _, sess := range m {
		out = append(out, sess)
	}
	return out
}

// UpdateSessionState updates the in-memory session state for the given tenant.
// Terminal states stamp CompletedAt.
func (s *State) UpdateSessionState(t tenant.ID, sessionID, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.tenantSessions(t)[sessionID]
	if !ok {
		return
	}
	sess.State = state
	if state == "completed" || state == "failed" || state == "cancelled" || state == "canceled" {
		now := time.Now().UTC()
		sess.CompletedAt = &now
	}
}

// SetSessionResult stores the final result on a session for the given tenant.
func (s *State) SetSessionResult(t tenant.ID, sessionID string, result *runtime.Result) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.tenantSessions(t)[sessionID]
	if !ok {
		return
	}
	sess.Result = result
}

// AttachBroadcaster returns the existing broadcaster for a session or creates
// a new one. Broadcasters are keyed by session ID, which is already unique
// across tenants.
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
