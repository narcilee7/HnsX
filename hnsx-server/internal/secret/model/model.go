// Package model defines the Secret aggregate for the HnsX control plane.
//
// Secrets are never stored in DomainSpec. They are referenced by name via
// `${secret:name}` placeholders and resolved at runtime by the secret service.
package model

import "errors"

// Secret is a named credential or sensitive value.
type Secret struct {
	ID    string
	Name  string
	Value string
	Kind  string // e.g. "api_key", "token", "password"
}

// Common errors returned by the secret service.
var (
	ErrSecretNotFound = errors.New("secret: not found")
	ErrInvalidName    = errors.New("secret: invalid name")
)
