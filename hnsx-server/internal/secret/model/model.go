// Package model defines the Secret aggregate for the HnsX control plane.
//
// Secrets are never stored in DomainSpec. They are referenced by name via
// `${secret:name}` placeholders and resolved at runtime by the secret
// service. The Value field, when set, is *envelope-encrypted*; the
// API surface never returns it.
//
// Fingerprint is a stable "****xxxx" preview that operators can match
// against typed values without exposing the plaintext.
package model

import (
	"errors"
	"time"
)

// Secret is a named credential or sensitive value.
type Secret struct {
	ID          string
	Name        string
	Value       string // envelope-encrypted ciphertext (base64)
	PlainValue  string // only populated when the caller explicitly decrypts
	Description string
	Kind        string // e.g. "api_key", "token", "password"
	Fingerprint string // last-4 of ciphertext, safe for wire
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ListItem is what the control plane returns to the API. It deliberately
// drops Value/PlainValue; clients that need plaintext call a separate
// resolve-on-render path that the audit log can attribute.
type ListItem struct {
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Kind        string    `json:"kind,omitempty"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Common errors returned by the secret service.
var (
	ErrSecretNotFound = errors.New("secret: not found")
	ErrInvalidName    = errors.New("secret: invalid name")
	ErrSecretExists   = errors.New("secret: name already exists")
)
