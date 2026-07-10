// Package repository defines the domain.Repository contract and its
// in-memory implementation. The Postgres implementation lives in the same
// package (postgres.go).
package repository

import (
	"sync"

	"github.com/hnsx-io/hnsx/server/internal/domain/model"
)

// Repository is the persistence contract for RegisteredDomain aggregates.
// Implementations:
//   - InMemoryRepository (tests / no-db mode)
//   - PostgresRepository (production, tables `domains` + `domain_versions`)
type Repository interface {
	// Save persists a domain. If a domain with the same ID already exists it
	// is overwritten (upsert semantics).
	Save(d *model.RegisteredDomain) error

	// ByID returns the domain with the given ID, or ErrDomainNotFound.
	ByID(id string) (*model.RegisteredDomain, error)

	// All returns every registered domain. The order is undefined; callers
	// should sort by ID or CreatedAt themselves.
	All() ([]*model.RegisteredDomain, error)

	// Delete removes a domain by ID. Deleting a non-existent domain is a
	// no-op and returns nil.
	Delete(id string) error

	// Exists reports whether a domain with the given ID is registered.
	Exists(id string) (bool, error)
}

// InMemoryRepository is a thread-safe in-memory implementation of Repository.
// It is used for Phase 1 and tests.
type InMemoryRepository struct {
	mu      sync.RWMutex
	domains map[string]*model.RegisteredDomain
}

// NewInMemoryRepository constructs an empty in-memory repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{domains: map[string]*model.RegisteredDomain{}}
}

// Save implements Repository.
func (r *InMemoryRepository) Save(d *model.RegisteredDomain) error {
	if d == nil {
		return model.ErrInvalidSpec
	}
	if err := d.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.domains[d.ID] = d
	return nil
}

// ByID implements Repository.
func (r *InMemoryRepository) ByID(id string) (*model.RegisteredDomain, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.domains[id]
	if !ok {
		return nil, model.ErrDomainNotFound
	}
	return d, nil
}

// All implements Repository.
func (r *InMemoryRepository) All() ([]*model.RegisteredDomain, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*model.RegisteredDomain, 0, len(r.domains))
	for _, d := range r.domains {
		out = append(out, d)
	}
	return out, nil
}

// Delete implements Repository.
func (r *InMemoryRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.domains, id)
	return nil
}

// Exists implements Repository.
func (r *InMemoryRepository) Exists(id string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.domains[id]
	return ok, nil
}

var _ Repository = (*InMemoryRepository)(nil)
