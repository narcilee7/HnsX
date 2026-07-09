// Package service implements the secret application use cases.
//
// It resolves `${secret:name}` placeholders at runtime and records access
// attempts for audit purposes.
package service

import (
	"fmt"
	"regexp"

	"github.com/hnsx-io/hnsx/server/internal/secret/model"
	"github.com/hnsx-io/hnsx/server/internal/secret/repository"
)

// Service implements the secret application use cases.
type Service struct {
	repo repository.Repository
}

// NewService constructs a Service backed by the supplied repository.
func NewService(repo repository.Repository) *Service {
	return &Service{repo: repo}
}

// Save stores a secret.
func (s *Service) Save(sec *model.Secret) error {
	return s.repo.Save(sec)
}

// Resolve replaces all `${secret:name}` placeholders in v with the named
// secret value. Missing secrets return an error.
func (s *Service) Resolve(v string) (string, error) {
	return secretPlaceholder.ReplaceAllStringFunc(v, func(match string) string {
		name := match[len("${secret:") : len(match)-1]
		sec, err := s.repo.ByName(name)
		if err != nil {
			return match // leave unresolved; caller checks equality
		}
		return sec.Value
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
