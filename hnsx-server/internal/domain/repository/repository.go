// Package repository defines the domain.Repository contract and its
// in-memory implementation. The Postgres implementation lives in the same
// package (postgres.go).
package repository

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// Repository is the persistence contract for RegisteredDomain aggregates.
// Implementations:
//   - InMemoryRepository (tests / no-db mode)
//   - PostgresRepository (production, tables `domains` + `domain_versions`)
type Repository interface {
	// Save persists a domain scoped to a tenant.
	Save(tenantID tenant.ID, d *model.RegisteredDomain) error

	// ByID returns the domain with the given ID scoped to a tenant, or ErrDomainNotFound.
	ByID(tenantID tenant.ID, id string) (*model.RegisteredDomain, error)

	// All returns every registered domain for a tenant.
	All(tenantID tenant.ID) ([]*model.RegisteredDomain, error)

	// Delete removes a domain by ID scoped to a tenant.
	Delete(tenantID tenant.ID, id string) error

	// Exists reports whether a domain with the given ID is registered for a tenant.
	Exists(tenantID tenant.ID, id string) (bool, error)

	// ListVersions returns every stored version for a domain, newest first.
	ListVersions(tenantID tenant.ID, id string) ([]VersionRecord, error)

	// GetVersion returns the spec for a specific domain version.
	GetVersion(tenantID tenant.ID, id, version string) (*spec.DomainSpec, error)
}

// VersionRecord is a single persisted version of a DomainSpec.
type VersionRecord struct {
	Version   string
	CreatedAt time.Time
	Spec      *spec.DomainSpec
}

// InMemoryRepository is a thread-safe in-memory implementation of Repository.
// It is used for Phase 1 and tests.
type InMemoryRepository struct {
	mu      sync.RWMutex
	domains map[string]*model.RegisteredDomain
	history map[string][]VersionRecord
}

// NewInMemoryRepository constructs an empty in-memory repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		domains: map[string]*model.RegisteredDomain{},
		history: map[string][]VersionRecord{},
	}
}

// Save implements Repository.
func (r *InMemoryRepository) Save(tenantID tenant.ID, d *model.RegisteredDomain) error {
	_ = tenantID
	if d == nil {
		return model.ErrInvalidSpec
	}
	if err := d.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.domains[d.ID] = d
	r.history[d.ID] = append(r.history[d.ID], VersionRecord{
		Version:   d.Version,
		CreatedAt: time.Now().UTC(),
		Spec:      cloneSpec(d.Spec),
	})
	return nil
}

// ByID implements Repository.
func (r *InMemoryRepository) ByID(tenantID tenant.ID, id string) (*model.RegisteredDomain, error) {
	_ = tenantID
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.domains[id]
	if !ok {
		return nil, model.ErrDomainNotFound
	}
	return d, nil
}

// All implements Repository.
func (r *InMemoryRepository) All(tenantID tenant.ID) ([]*model.RegisteredDomain, error) {
	_ = tenantID
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*model.RegisteredDomain, 0, len(r.domains))
	for _, d := range r.domains {
		out = append(out, d)
	}
	return out, nil
}

// Delete implements Repository.
func (r *InMemoryRepository) Delete(tenantID tenant.ID, id string) error {
	_ = tenantID
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.domains, id)
	delete(r.history, id)
	return nil
}

// Exists implements Repository.
func (r *InMemoryRepository) Exists(tenantID tenant.ID, id string) (bool, error) {
	_ = tenantID
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.domains[id]
	return ok, nil
}

// ListVersions implements Repository.
func (r *InMemoryRepository) ListVersions(tenantID tenant.ID, id string) ([]VersionRecord, error) {
	_ = tenantID
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.domains[id]; !ok {
		return nil, model.ErrDomainNotFound
	}
	all := r.history[id]
	out := make([]VersionRecord, len(all))
	for i := len(all) - 1; i >= 0; i-- {
		out[len(all)-1-i] = all[i]
	}
	return out, nil
}

// GetVersion implements Repository.
func (r *InMemoryRepository) GetVersion(tenantID tenant.ID, id, version string) (*spec.DomainSpec, error) {
	_ = tenantID
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, rec := range r.history[id] {
		if rec.Version == version {
			return rec.Spec, nil
		}
	}
	return nil, model.ErrDomainNotFound
}

func cloneSpec(s *spec.DomainSpec) *spec.DomainSpec {
	if s == nil {
		return nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil
	}
	var out spec.DomainSpec
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return &out
}

var _ Repository = (*InMemoryRepository)(nil)
