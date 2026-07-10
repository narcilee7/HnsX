// Package repository defines the policy.Repository contract and its
// in-memory implementation.
package repository

import (
	"errors"
	"sync"

	"github.com/hnsx-io/hnsx/server/internal/policy/model"
)

// Repository is the persistence contract for Policy aggregates.
type Repository interface {
	Save(p *model.Policy) error
	ByDomain(domainID string) (*model.Policy, error)
}

// InMemoryRepository is a thread-safe in-memory implementation.
type InMemoryRepository struct {
	mu      sync.RWMutex
	policies map[string]*model.Policy
}

// NewInMemoryRepository constructs an empty repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{policies: map[string]*model.Policy{}}
}

// Save implements Repository.
func (r *InMemoryRepository) Save(p *model.Policy) error {
	if p == nil || p.DomainID == "" {
		return errors.New("policy: invalid policy")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policies[p.DomainID] = p
	return nil
}

// ByDomain implements Repository.
func (r *InMemoryRepository) ByDomain(domainID string) (*model.Policy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.policies[domainID]
	if !ok {
		return nil, model.ErrPolicyNotFound
	}
	return p, nil
}

var _ Repository = (*InMemoryRepository)(nil)
