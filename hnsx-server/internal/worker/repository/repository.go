// Package repository defines the worker.Repository contract and its
// in-memory implementation. The Postgres implementation lives in the same
// package (postgres.go).
package repository

import (
	"sync"

	"github.com/hnsx-io/hnsx/server/internal/worker/model"
)

// Repository is the persistence contract for Worker aggregates.
// Implementations:
//   - InMemoryRepository (tests / no-db mode)
//   - PostgresRepository (production, table `runtimes`)
type Repository interface {
	// Save persists a worker. If a worker with the same ID already exists it
	// is overwritten (upsert semantics).
	Save(w *model.Worker) error

	// ByID returns the worker with the given ID, or ErrWorkerNotFound.
	ByID(id string) (*model.Worker, error)

	// All returns every registered worker. The order is undefined.
	All() ([]*model.Worker, error)

	// Delete removes a worker by ID. Deleting a non-existent worker is a
	// no-op and returns nil.
	Delete(id string) error
}

// InMemoryRepository is a thread-safe in-memory implementation of Repository.
type InMemoryRepository struct {
	mu      sync.RWMutex
	workers map[string]*model.Worker
}

// NewInMemoryRepository constructs an empty in-memory worker repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{workers: map[string]*model.Worker{}}
}

// Save implements Repository.
func (r *InMemoryRepository) Save(w *model.Worker) error {
	if w == nil || w.ID == "" {
		return model.ErrWorkerNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workers[w.ID] = w
	return nil
}

// ByID implements Repository.
func (r *InMemoryRepository) ByID(id string) (*model.Worker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.workers[id]
	if !ok {
		return nil, model.ErrWorkerNotFound
	}
	return w, nil
}

// All implements Repository.
func (r *InMemoryRepository) All() ([]*model.Worker, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*model.Worker, 0, len(r.workers))
	for _, w := range r.workers {
		out = append(out, w)
	}
	return out, nil
}

// Delete implements Repository.
func (r *InMemoryRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.workers, id)
	return nil
}

var _ Repository = (*InMemoryRepository)(nil)
