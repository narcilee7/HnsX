// Package store provides context/knowledge/ephemeral storage backends for
// HnsX sessions.
//
// The DomainSpec field ``store`` declares which backends a domain wants:
//
//	store:
//	  context:
//	    backend: in_memory
//	  knowledge:
//	    backend: postgres
//
// Backends are intentionally simple key/value surfaces. Higher-level
// semantics (conversation threading, vector search, TTL) live in the
// application layer.
package store

import "fmt"

// Namespace identifies the logical purpose of a stored value.
type Namespace string

const (
	// NamespaceContext holds short-term working memory for the current
	// session / turn.
	NamespaceContext Namespace = "context"
	// NamespaceKnowledge holds long-term, cross-session facts.
	NamespaceKnowledge Namespace = "knowledge"
	// NamespaceEphemeral holds temporary computation state.
	NamespaceEphemeral Namespace = "ephemeral"
)

// Backend is a namespace-aware key/value store.
type Backend interface {
	// Get returns a value from the named namespace.
	Get(ns Namespace, key string) (string, error)
	// Set stores a value in the named namespace.
	Set(ns Namespace, key, value string) error
	// Delete removes a key from the named namespace.
	Delete(ns Namespace, key string) error
}

// InMemoryBackend stores values in process memory. It is used for tests and
// no-db mode.
type InMemoryBackend struct {
	data map[Namespace]map[string]string
}

// NewInMemoryBackend creates an in-memory backend with all namespaces.
func NewInMemoryBackend() *InMemoryBackend {
	return &InMemoryBackend{
		data: map[Namespace]map[string]string{
			NamespaceContext:   {},
			NamespaceKnowledge: {},
			NamespaceEphemeral: {},
		},
	}
}

// Get implements Backend.
func (b *InMemoryBackend) Get(ns Namespace, key string) (string, error) {
	if b.data[ns] == nil {
		return "", nil
	}
	return b.data[ns][key], nil
}

// Set implements Backend.
func (b *InMemoryBackend) Set(ns Namespace, key, value string) error {
	if b.data[ns] == nil {
		b.data[ns] = map[string]string{}
	}
	b.data[ns][key] = value
	return nil
}

// Delete implements Backend.
func (b *InMemoryBackend) Delete(ns Namespace, key string) error {
	if b.data[ns] != nil {
		delete(b.data[ns], key)
	}
	return nil
}

// Config selects a backend implementation and its per-namespace settings.
type Config struct {
	Context   BackendConfig `json:"context" yaml:"context"`
	Knowledge BackendConfig `json:"knowledge" yaml:"knowledge"`
	Ephemeral BackendConfig `json:"ephemeral" yaml:"ephemeral"`
}

// BackendConfig is one namespace's backend declaration.
type BackendConfig struct {
	Backend string         `json:"backend" yaml:"backend"`
	Config  map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
}

// NewBackend constructs a Backend from a store Config. Unknown backends fall
// back to InMemoryBackend with a warning.
func NewBackend(cfg Config) (Backend, error) {
	// Phase 1 only supports in_memory. Future PRs add postgres/redis.
	for ns, bc := range map[Namespace]BackendConfig{
		NamespaceContext:   cfg.Context,
		NamespaceKnowledge: cfg.Knowledge,
		NamespaceEphemeral: cfg.Ephemeral,
	} {
		if bc.Backend != "" && bc.Backend != "in_memory" {
			return nil, fmt.Errorf("store namespace %q backend %q is not supported in this build", ns, bc.Backend)
		}
	}
	return NewInMemoryBackend(), nil
}
