// Package repository defines the audit.Repository contract and its
// in-memory implementation.
package repository

import (
	"errors"
	"sort"
	"sync"

	"github.com/hnsx-io/hnsx/server/internal/audit/model"
)

// Repository is the persistence contract for audit entries.
type Repository interface {
	Save(e *model.Entry) error
	BySession(sessionID string) ([]model.Entry, error)
	ByDomain(domainID string) ([]model.Entry, error)
	List(limit, offset int) ([]model.Entry, int, error)
}

// InMemoryRepository is a thread-safe in-memory implementation.
type InMemoryRepository struct {
	mu      sync.RWMutex
	entries []model.Entry
}

// NewInMemoryRepository constructs an empty repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{entries: []model.Entry{}}
}

// Save implements Repository.
func (r *InMemoryRepository) Save(e *model.Entry) error {
	if e == nil {
		return errors.New("audit: nil entry")
	}
	if err := e.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, *e)
	return nil
}

// BySession implements Repository.
func (r *InMemoryRepository) BySession(sessionID string) ([]model.Entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.Entry, 0)
	for _, e := range r.entries {
		if e.SessionID == sessionID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	return out, nil
}

// ByDomain implements Repository.
func (r *InMemoryRepository) ByDomain(domainID string) ([]model.Entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.Entry, 0)
	for _, e := range r.entries {
		if e.DomainID == domainID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Timestamp.After(out[j].Timestamp)
	})
	return out, nil
}

// List implements Repository with newest-first pagination.
func (r *InMemoryRepository) List(limit, offset int) ([]model.Entry, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	total := len(r.entries)
	sorted := make([]model.Entry, total)
	copy(sorted, r.entries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.After(sorted[j].Timestamp)
	})
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return sorted[offset:end], total, nil
}

var _ Repository = (*InMemoryRepository)(nil)
