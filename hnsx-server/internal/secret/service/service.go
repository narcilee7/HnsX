// Package service implements the secret application use cases.
//
// It resolves `${secret:name}` placeholders at runtime and records access
// attempts for audit purposes. Values are AES-GCM encrypted at rest; the
// service holds a Cipher dependency and refuses to operate without one.
package service

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/secret/crypto"
	"github.com/hnsx-io/hnsx/server/internal/secret/model"
	"github.com/hnsx-io/hnsx/server/internal/secret/repository"
)

// Service implements the secret application use cases.
type Service struct {
	repo   repository.Repository
	cipher *crypto.Cipher
	now    func() time.Time
}

// NewService constructs a Service backed by the supplied repository and
// cipher. The cipher is mandatory — passing nil panics at write time so
// misconfiguration is loud, not silent.
func NewService(repo repository.Repository, cipher *crypto.Cipher) *Service {
	return &Service{repo: repo, cipher: cipher, now: time.Now}
}

// SetClock replaces the time source (test-only).
func (s *Service) SetClock(fn func() time.Time) { s.now = fn }

// Cipher returns the active cipher (for tests / debugging).
func (s *Service) Cipher() *crypto.Cipher { return s.cipher }

// Save encrypts plaintext and persists the resulting envelope. The
// incoming Secret must carry PlainValue; Value on the caller's struct
// is overwritten with the envelope before Save returns.
func (s *Service) Save(sec *model.Secret) error {
	if sec == nil || sec.Name == "" {
		return model.ErrInvalidName
	}
	if s.cipher == nil {
		return errors.New("secret.Service: cipher not configured")
	}
	if sec.Kind == "" {
		sec.Kind = "generic"
	}
	envelope, err := s.cipher.Encrypt(sec.PlainValue)
	if err != nil {
		return fmt.Errorf("secret: encrypt: %w", err)
	}
	sec.Value = envelope
	sec.Fingerprint = crypto.Fingerprint(envelope)
	if sec.PlainValue != "" {
		sec.UpdatedAt = s.now().UTC()
	}
	if sec.CreatedAt.IsZero() {
		sec.CreatedAt = sec.UpdatedAt
	}
	sec.PlainValue = "" // never store plaintext on the returned struct
	return s.repo.Save(sec)
}

// List returns metadata for every secret. The wire payload contains no
// Value / PlainValue — callers that need plaintext must go through
// Resolve so the access is auditable.
func (s *Service) List() ([]model.ListItem, error) {
	return s.repo.List()
}

// ByName returns the encrypted record (Value = envelope). Used by the
// resolve path; never return to the API directly.
func (s *Service) ByName(name string) (*model.Secret, error) {
	return s.repo.ByName(name)
}

// Delete removes a secret. Idempotent.
func (s *Service) Delete(name string) error {
	return s.repo.Delete(name)
}

// Resolve replaces all `${secret:name}` placeholders in v with the named
// secret's plaintext. Missing secrets leave the placeholder intact so the
// caller can detect and surface a "not found" error.
func (s *Service) Resolve(v string) (string, error) {
	if s.cipher == nil {
		return "", errors.New("secret.Service: cipher not configured")
	}
	return secretPlaceholder.ReplaceAllStringFunc(v, func(match string) string {
		name := match[len("${secret:") : len(match)-1]
		sec, err := s.repo.ByName(name)
		if err != nil {
			return match
		}
		plain, err := s.cipher.Decrypt(sec.Value)
		if err != nil {
			return match
		}
		return plain
	}), nil
}

// ResolveMap applies Resolve to every string value in a map recursively.
func (s *Service) ResolveMap(input map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(input))
	for k, v := range input {
		resolved, err := s.resolveValue(v)
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", k, err)
		}
		out[k] = resolved
	}
	return out, nil
}

func (s *Service) resolveValue(v any) (any, error) {
	switch val := v.(type) {
	case string:
		resolved, err := s.Resolve(val)
		if err != nil {
			return nil, err
		}
		if resolved == val && secretPlaceholder.MatchString(val) {
			name := val[len("${secret:") : len(val)-1]
			return nil, fmt.Errorf("%w: %s", model.ErrSecretNotFound, name)
		}
		return resolved, nil
	case map[string]any:
		return s.ResolveMap(val)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			r, err := s.resolveValue(item)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	default:
		return v, nil
	}
}

var secretPlaceholder = regexp.MustCompile(`\$\{secret:([^}]+)\}`)
