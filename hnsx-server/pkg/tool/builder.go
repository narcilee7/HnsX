package tool

import (
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// BuildFromSpec creates a Tool Registry populated from the tools declared in
// a DomainSpec. Phase 1 registers placeholder tools; real tool implementations
// (http, sql, shell, python) are wired in during Phase 4 follow-ups.
func BuildFromSpec(s *spec.DomainSpec) *Registry {
	r := NewRegistry()
	if s == nil {
		return r
	}
	for name := range s.Harness.Tools {
		r.Register(NewPlaceholderTool(name))
	}
	return r
}

// RequiredSecretNames returns the secret names referenced by tool configs.
func RequiredSecretNames(s *spec.DomainSpec) []string {
	if s == nil {
		return nil
	}
	seen := map[string]struct{}{}
	for _, tc := range s.Harness.Tools {
		// Tool config is opaque at this layer; future PRs introspect known
		// schemas. For now we rely on the secret service to scan resolved
		// strings at runtime.
		_ = tc
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	return out
}
