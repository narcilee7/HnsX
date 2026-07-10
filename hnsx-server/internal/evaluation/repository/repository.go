// Package repository defines the evaluation.Repository contract and its
// in-memory implementation.
package repository

import (
	"errors"
	"sync"

	"github.com/hnsx-io/hnsx/server/internal/evaluation/model"
)

// Repository is the persistence contract for EvalSet / EvalRun aggregates.
type Repository interface {
	SaveSet(set *model.EvalSet) error
	SetByID(id string) (*model.EvalSet, error)
	SetsByDomain(domainID string) ([]model.EvalSet, error)
	ListSets(limit, offset int) ([]model.EvalSet, int, error)

	SaveRun(run *model.EvalRun) error
	RunByID(id string) (*model.EvalRun, error)
	RunsBySet(setID string) ([]model.EvalRun, error)
}

// InMemoryRepository is a thread-safe in-memory implementation.
type InMemoryRepository struct {
	mu      sync.RWMutex
	sets    map[string]*model.EvalSet
	runs    map[string]*model.EvalRun
}

// NewInMemoryRepository constructs an empty repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		sets: map[string]*model.EvalSet{},
		runs: map[string]*model.EvalRun{},
	}
}

// SaveSet implements Repository.
func (r *InMemoryRepository) SaveSet(set *model.EvalSet) error {
	if set == nil || set.ID == "" {
		return errors.New("evaluation: invalid set")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sets[set.ID] = set
	return nil
}

// SetByID implements Repository.
func (r *InMemoryRepository) SetByID(id string) (*model.EvalSet, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sets[id]
	if !ok {
		return nil, model.ErrEvalSetNotFound
	}
	return s, nil
}

// SetsByDomain implements Repository.
func (r *InMemoryRepository) SetsByDomain(domainID string) ([]model.EvalSet, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.EvalSet, 0)
	for _, s := range r.sets {
		if s.DomainID == domainID {
			out = append(out, *s)
		}
	}
	return out, nil
}

// ListSets implements Repository.
func (r *InMemoryRepository) ListSets(limit, offset int) ([]model.EvalSet, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	total := len(r.sets)
	out := make([]model.EvalSet, 0, total)
	for _, s := range r.sets {
		out = append(out, *s)
	}
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return out[offset:end], total, nil
}

// SaveRun implements Repository.
func (r *InMemoryRepository) SaveRun(run *model.EvalRun) error {
	if run == nil || run.ID == "" {
		return errors.New("evaluation: invalid run")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[run.ID] = run
	return nil
}

// RunByID implements Repository.
func (r *InMemoryRepository) RunByID(id string) (*model.EvalRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.runs[id]
	if !ok {
		return nil, model.ErrEvalRunNotFound
	}
	return run, nil
}

// RunsBySet implements Repository.
func (r *InMemoryRepository) RunsBySet(setID string) ([]model.EvalRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.EvalRun, 0)
	for _, run := range r.runs {
		if run.EvalSetID == setID {
			out = append(out, *run)
		}
	}
	return out, nil
}

var _ Repository = (*InMemoryRepository)(nil)
