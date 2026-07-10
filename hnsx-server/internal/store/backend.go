// Package store implements the Store bounded context for HnsX.
//
// It provides namespace-aware key/value storage (context, knowledge,
// ephemeral) with swappable backends: in-memory for tests/no-db mode and
// Postgres for production persistence. The DomainSpec ``store`` block selects
// the backend per namespace.
package store

import "errors"

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

// Common store errors.
var (
	ErrNotFound     = errors.New("store: key not found")
	ErrInvalidKey   = errors.New("store: invalid key")
	ErrInvalidValue = errors.New("store: invalid value")
)

// Backend is a namespace-aware key/value store.
type Backend interface {
	// Get returns a value from the named namespace.
	// An empty string with a nil error means the key is absent.
	Get(ns Namespace, key string) (string, error)
	// Set stores a value in the named namespace.
	Set(ns Namespace, key, value string) error
	// Delete removes a key from the named namespace.
	Delete(ns Namespace, key string) error
}
