// Package repository defines the secret.Repository contract and its in-memory
// implementation. The Postgres implementation lives in the same package
// (postgres.go).
package repository

import (
	"sync"

	"github.com/hnsx-io/hnsx/server/internal/secret/model"
)

// Repository is the persistence contract for Secret aggregates.
type Repository interface {
	Save(s *model.Secret) error
	ByName(name string) (*model.Secret, error)
	Delete(name string) error
}

// InMemoryRepository is a thread-safe in-memory implementation of Repository.
type InMemoryRepository struct {
	mu      sync.RWMutex
	secrets map[string]*model.Secret
}

// NewInMemoryRepository constructs an empty in-memory secret repository.
func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{secrets: map[string]*model.Secret{}}
}

// Save implements Repository.
func (r *InMemoryRepository) Save(s *model.Secret) error {
	if s == nil || s.Name == "" {
		return model.ErrInvalidName
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.secrets[s.Name] = s
	return nil
}

// ByName implements Repository.
func (r *InMemoryRepository) ByName(name string) (*model.Secret, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.secrets[name]
	if !ok {
		return nil, model.ErrSecretNotFound
	}
	return s, nil
}

// Delete implements Repository.
func (r *InMemoryRepository) Delete(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.secrets, name)
	return nil
}

var _ Repository = (*InMemoryRepository)(nil)
