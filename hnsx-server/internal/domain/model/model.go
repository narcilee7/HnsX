// Package model defines the Domain aggregate root for the HnsX control plane.
//
// A Domain is the unit of Harness configuration: it holds the DomainSpec,
// version metadata, and lifecycle timestamps.
package model

import (
	"errors"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// Common errors returned by the domain service and repository.
var (
	ErrDomainNotFound = errors.New("domain: not found")
	ErrDomainExists   = errors.New("domain: already exists")
	ErrInvalidSpec    = errors.New("domain: invalid spec")
)

// RegisteredDomain is the aggregate root for a loaded DomainSpec.
//
// It wraps the canonical spec with control-plane metadata (registration time,
// current version, description cache). The Spec field is the source of truth;
// the top-level ID/Version/Description fields are cached for listing/filtering.
type RegisteredDomain struct {
	ID          string
	Version     string
	Description string
	Spec        *spec.DomainSpec
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Validate returns a non-nil error if the registered domain cannot be served.
// Phase 1 only checks structural non-nil requirements; future phases will add
// policy/budget/agent-reference validation.
func (d *RegisteredDomain) Validate() error {
	if d == nil {
		return ErrInvalidSpec
	}
	if d.ID == "" {
		return ErrInvalidSpec
	}
	if d.Spec == nil {
		return ErrInvalidSpec
	}
	return nil
}

// Harness returns the Harness block from the underlying spec, or nil.
func (d *RegisteredDomain) Harness() any {
	if d == nil || d.Spec == nil {
		return nil
	}
	return d.Spec.Harness
}
