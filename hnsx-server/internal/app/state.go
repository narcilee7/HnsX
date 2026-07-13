// Package app holds the in-process broadcaster index shared by the HTTP API
// and the gRPC control plane.
//
// It deliberately does NOT contain protocol concerns (chi, gRPC handlers, SSE
// serialization). Domain and session authoritative state live in their
// respective services and repositories; this package only manages the
// per-session broadcaster pub/sub used for SSE fan-out.
package app

import (
	"context"
	"sync"

	"github.com/hnsx-io/hnsx/server/internal/session/broadcaster"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// RegisteredDomain is the runtime view of a loaded DomainSpec.
//
// It is kept as an API-layer view model so that HTTP handlers do not need to
// import internal/domain/model directly. The authoritative aggregate lives in
// internal/domain/model.
type RegisteredDomain struct {
	ID          string
	Version     string
	Description string
	Spec        *domain.DomainSpec
	Harness     any
	CreatedAt   string
	UpdatedAt   string
}

// RegisteredSession is the runtime metadata for one Session run.
//
// It is kept as an API-layer view model so that HTTP handlers do not need to
// import internal/session/model directly. The authoritative aggregate lives in
// internal/session/model.
type RegisteredSession struct {
	ID            string
	DomainID      string
	DomainVersion string
	Orchestration string
	State         string
	Trigger       map[string]any
	Result        *domain.Result
	StartedAt     string
	CompletedAt   *string
}

// State holds the in-process broadcaster index. It is NOT the source of truth
// for domain or session aggregates; those are owned by their services.
type State struct {
	mu           sync.RWMutex
	broadcasters map[string]*broadcaster.Broadcaster
}

// NewState constructs an empty broadcaster index.
func NewState() *State {
	return &State{
		broadcasters: map[string]*broadcaster.Broadcaster{},
	}
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
// broadcaster. Returns false if publishing fails.
func (s *State) PublishObservation(sessionID string, obs domain.Observation) bool {
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
