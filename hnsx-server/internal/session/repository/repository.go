// Package repository defines the session.Repository contract and its
// in-memory implementation. The Postgres implementation lives in the same
// package (postgres.go).
package repository

import (
	"sync"

	"github.com/hnsx-io/hnsx/server/internal/session/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
)

// Repository is the persistence contract for Session aggregates.
// Implementations:
//   - InMemoryRepository (tests / no-db mode)
//   - PostgresRepository (production, table `sessions`)
type Repository interface {
	// Save persists a session scoped to a tenant.
	Save(tenantID tenant.ID, s *model.Session) error

	// ByID returns the session with the given ID scoped to a tenant, or ErrSessionNotFound.
	ByID(tenantID tenant.ID, id string) (*model.Session, error)

	// All returns every session for a tenant.
	All(tenantID tenant.ID) ([]*model.Session, error)

	// ByDomain returns every session for a given domain ID scoped to a tenant.
	ByDomain(tenantID tenant.ID, domainID string) ([]*model.Session, error)

	// ListByState returns every session in the given state scoped to a tenant.
	ListByState(tenantID tenant.ID, state model.State) ([]*model.Session, error)

	// Delete removes a session by ID scoped to a tenant.
	Delete(tenantID tenant.ID, id string) error
}

// InMemoryRepository is a thread-safe in-memory implementation of Repository.
// It is used for tests and no-db mode.
type InMemoryRepository struct {
	mu       sync.RWMutex
	sessions map[string]*model.Session
}

// NewInMemoryRepository constructs an empty in-memory session repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{sessions: map[string]*model.Session{}}
}

// Save implements Repository.
func (r *InMemoryRepository) Save(tenantID tenant.ID, s *model.Session) error {
	_ = tenantID
	if s == nil || s.ID == "" {
		return model.ErrInvalidSession
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[s.ID] = s
	return nil
}

// ByID implements Repository.
func (r *InMemoryRepository) ByID(tenantID tenant.ID, id string) (*model.Session, error) {
	_ = tenantID
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sessions[id]
	if !ok {
		return nil, model.ErrSessionNotFound
	}
	return s, nil
}

// All implements Repository.
func (r *InMemoryRepository) All(tenantID tenant.ID) ([]*model.Session, error) {
	_ = tenantID
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*model.Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, s)
	}
	return out, nil
}

// ByDomain implements Repository.
func (r *InMemoryRepository) ByDomain(tenantID tenant.ID, domainID string) ([]*model.Session, error) {
	_ = tenantID
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*model.Session, 0)
	for _, s := range r.sessions {
		if s.DomainID == domainID {
			out = append(out, s)
		}
	}
	return out, nil
}

// ListByState implements Repository.
func (r *InMemoryRepository) ListByState(tenantID tenant.ID, state model.State) ([]*model.Session, error) {
	_ = tenantID
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*model.Session, 0)
	for _, s := range r.sessions {
		if s.State == state {
			out = append(out, s)
		}
	}
	return out, nil
}

// Delete implements Repository.
func (r *InMemoryRepository) Delete(tenantID tenant.ID, id string) error {
	_ = tenantID
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, id)
	return nil
}

var _ Repository = (*InMemoryRepository)(nil)
var _ Repository = (*PostgresRepository)(nil)
