// Package repository defines the policy.Repository contract and its
// in-memory implementation.
package repository

import (
	"sort"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/policy/model"
)

// Repository is the persistence contract for Policy aggregates.
//
// Policies are looked up by ID. The Domain scoping that previously lived
// in `p.DomainID` is now stored as a separate `BoundDomain` field —
// keeping it 1:1 today (no multi-domain binding yet) lets the table
// migrate to a link table later without an API break.
type Repository interface {
	Save(p *model.Policy) error
	ByID(id string) (*model.Policy, error)
	List() ([]model.ListItem, error)
	Delete(id string) error
	// BindDomain associates an existing policy with a domain. Pass
	// empty domainID to unbind. Implementation enforces one policy
	// per domain in this revision.
	BindDomain(id, domainID string) error
	// ByDomain returns whatever policy is bound to domainID, or
	// model.ErrPolicyNotFound if no binding exists.
	ByDomain(domainID string) (*model.Policy, error)
}

// InMemoryRepository is a thread-safe in-memory implementation.
type InMemoryRepository struct {
	mu           sync.RWMutex
	policies     map[string]*model.Policy // id -> policy
	bindingIndex map[string]string        // domain -> policy id
}

// NewInMemoryRepository constructs an empty repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		policies:     map[string]*model.Policy{},
		bindingIndex: map[string]string{},
	}
}

// Save implements Repository.
func (r *InMemoryRepository) Save(p *model.Policy) error {
	if p == nil || p.ID == "" {
		return model.ErrInvalidPolicyID
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	if existing, ok := r.policies[p.ID]; ok {
		p.CreatedAt = existing.CreatedAt
	} else {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	r.policies[p.ID] = p
	if p.BoundDomain != "" {
		r.bindingIndex[p.BoundDomain] = p.ID
	}
	return nil
}

// ByID implements Repository.
func (r *InMemoryRepository) ByID(id string) (*model.Policy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.policies[id]
	if !ok {
		return nil, model.ErrPolicyNotFound
	}
	// Return a copy so the caller can't mutate the record under us.
	copy := *p
	return &copy, nil
}

// List implements Repository.
func (r *InMemoryRepository) List() ([]model.ListItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.ListItem, 0, len(r.policies))
	ids := make([]string, 0, len(r.policies))
	for id := range r.policies {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := r.policies[id]
		out = append(out, model.ListItem{
			ID:          p.ID,
			Name:        p.Name,
			Description: p.Description,
			BoundDomain: p.BoundDomain,
			Budget:      p.Budget,
			Permissions: p.Permissions,
			Guardrails:  p.Guardrails,
			CreatedAt:   p.CreatedAt,
			UpdatedAt:   p.UpdatedAt,
		})
	}
	return out, nil
}

// Delete implements Repository.
func (r *InMemoryRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.policies[id]
	if !ok {
		return model.ErrPolicyNotFound
	}
	if p.BoundDomain != "" {
		delete(r.bindingIndex, p.BoundDomain)
	}
	delete(r.policies, id)
	return nil
}

// BindDomain implements Repository.
func (r *InMemoryRepository) BindDomain(id, domainID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.policies[id]
	if !ok {
		return model.ErrPolicyNotFound
	}
	if domainID == "" {
		if p.BoundDomain != "" {
			delete(r.bindingIndex, p.BoundDomain)
		}
		p.BoundDomain = ""
		p.UpdatedAt = time.Now().UTC()
		return nil
	}
	// Enforce 1:1: if the target domain is already bound to another
	// policy, drop that binding first so the contract is preserved.
	if existingID, bound := r.bindingIndex[domainID]; bound && existingID != id {
		if other, ok := r.policies[existingID]; ok {
			other.BoundDomain = ""
			other.UpdatedAt = time.Now().UTC()
		}
	}
	if prev, ok := r.bindingIndex[domainID]; ok && prev != id {
		if old, ok2 := r.policies[prev]; ok2 {
			old.BoundDomain = ""
			old.UpdatedAt = time.Now().UTC()
		}
	}
	p.BoundDomain = domainID
	p.UpdatedAt = time.Now().UTC()
	r.bindingIndex[domainID] = id
	return nil
}

// ByDomain implements Repository.
func (r *InMemoryRepository) ByDomain(domainID string) (*model.Policy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.bindingIndex[domainID]
	if !ok {
		return nil, model.ErrPolicyNotFound
	}
	p, ok := r.policies[id]
	if !ok {
		return nil, model.ErrPolicyNotFound
	}
	copy := *p
	return &copy, nil
}

var _ Repository = (*InMemoryRepository)(nil)
