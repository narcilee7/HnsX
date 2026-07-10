// Package repository defines the secret.Repository contract and its in-memory
// implementation. The Postgres implementation lives in the same package
// (postgres.go).
package repository

import (
	"errors"
	"sync"

	"github.com/hnsx-io/hnsx/server/internal/secret/model"
)

// Repository is the persistence contract for Secret aggregates.
//
// Save is upsert; List returns metadata only (no Value); ByName returns
// the full record (including the envelope-encrypted Value) so the
// service layer can decrypt it on the resolve path.
type Repository interface {
	Save(s *model.Secret) error
	ByName(name string) (*model.Secret, error)
	List() ([]model.ListItem, error)
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

// List implements Repository. It returns one ListItem per secret with
// metadata only — Value is intentionally omitted because the in-memory
// cipher test fixture stores ciphertext, and the wire contract must be
// the same here as in Postgres.
func (r *InMemoryRepository) List() ([]model.ListItem, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.ListItem, 0, len(r.secrets))
	for _, s := range r.secrets {
		out = append(out, model.ListItem{
			Name:        s.Name,
			Description: s.Description,
			Kind:        s.Kind,
			Fingerprint: s.Fingerprint,
			CreatedAt:   s.CreatedAt,
			UpdatedAt:   s.UpdatedAt,
		})
	}
	return out, nil
}

// IsNotFound reports whether err is the package's ErrSecretNotFound.
func IsNotFound(err error) bool {
	return errors.Is(err, model.ErrSecretNotFound)
}

var _ Repository = (*InMemoryRepository)(nil)
