// Package commands implements server-side application use cases. These
// commands operate on repository-backed services and are consumed by the HTTP
// API and gRPC control plane, not by the CLI.
package commands

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/local"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

// RegisterResult is returned by RegisterDomain.
type RegisterResult struct {
	Domain    *app.RegisteredDomain
	CreatedAt time.Time
}

// RegisterDomain parses a DomainSpec from a reader and registers it in the
// application state. Returns ErrDomainExists if the ID is already registered.
func RegisterDomain(state *app.State, tenantID tenant.ID, r io.Reader, contentType string) (*RegisterResult, error) {
	if state == nil {
		return nil, errors.New("nil app state")
	}
	s, err := local.DecodeDomainSpec(r, contentType)
	if err != nil {
		return nil, err
	}

	if _, exists := state.LookupDomain(tenantID, s.ID); exists {
		return nil, NewDomainExistsError(s.ID)
	}

	now := time.Now().UTC()
	d := &app.RegisteredDomain{
		ID:          s.ID,
		Version:     s.Version,
		Description: s.Description,
		Spec:        s,
		Harness:     s.Harness,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	state.RegisterDomain(tenantID, d)
	return &RegisterResult{Domain: d, CreatedAt: now}, nil
}

// UpdateDomain replaces the fields of an existing registered domain with the
// parsed body. Returns ErrDomainNotFound / ErrIDMismatch as appropriate.
func UpdateDomain(state *app.State, tenantID tenant.ID, id string, r io.Reader, contentType string) (*app.RegisteredDomain, error) {
	if state == nil {
		return nil, errors.New("nil app state")
	}
	existing, ok := state.LookupDomain(tenantID, id)
	if !ok {
		return nil, ErrDomainNotFound
	}

	s, err := local.DecodeDomainSpec(r, contentType)
	if err != nil {
		return nil, err
	}
	if s.ID != id {
		return nil, ErrIDMismatch
	}

	existing.Version = s.Version
	existing.Description = s.Description
	existing.Spec = s
	existing.Harness = s.Harness
	existing.UpdatedAt = time.Now().UTC()
	state.RegisterDomain(tenantID, existing) // re-register to refresh map
	return existing, nil
}

// DeleteDomain removes a domain from the application state.
func DeleteDomain(state *app.State, tenantID tenant.ID, id string) error {
	if state == nil {
		return errors.New("nil app state")
	}
	if _, ok := state.LookupDomain(tenantID, id); !ok {
		return ErrDomainNotFound
	}
	state.DeleteDomain(tenantID, id)
	return nil
}

// TriggerParams contains the inputs needed to start a session.
type TriggerParams struct {
	SessionID     string
	DomainID      string
	DomainVersion string
	Orchestration string
	Trigger       map[string]any
}

// NewSessionID generates a session ID for the given domain.
var NewSessionID = runtime.NewSessionID

// TriggerSession creates a registered session from a domain and trigger.
func TriggerSession(state *app.State, tenantID tenant.ID, domain *app.RegisteredDomain, trigger map[string]any, newID func(string) string) (*app.RegisteredSession, error) {
	if state == nil {
		return nil, errors.New("nil app state")
	}
	if domain == nil || domain.Spec == nil {
		return nil, errors.New("nil domain")
	}
	id := newID(domain.ID)
	sess := &app.RegisteredSession{
		ID:            id,
		DomainID:      domain.ID,
		DomainVersion: domain.Version,
		Orchestration: domain.Spec.Harness.Session.Mode,
		State:         "pending",
		Trigger:       trigger,
		StartedAt:     time.Now().UTC(),
	}
	state.RegisterSession(tenantID, sess)
	return sess, nil
}

// CancelSession transitions a non-terminal session to cancelled.
func CancelSession(state *app.State, tenantID tenant.ID, id string) (*app.RegisteredSession, error) {
	if state == nil {
		return nil, errors.New("nil app state")
	}
	sess, ok := state.LookupSession(tenantID, id)
	if !ok {
		return nil, ErrSessionNotFound
	}
	if sess.State == "completed" || sess.State == "failed" {
		return nil, fmt.Errorf("session is already in terminal state %q", sess.State)
	}
	state.UpdateSessionState(tenantID, id, "canceled")
	state.DetachBroadcaster(id)
	return sess, nil
}

// RerunSession creates a new session reusing the trigger of an existing one.
func RerunSession(state *app.State, tenantID tenant.ID, prevID string, newID func(string) string) (*app.RegisteredSession, error) {
	if state == nil {
		return nil, errors.New("nil app state")
	}
	prev, ok := state.LookupSession(tenantID, prevID)
	if !ok {
		return nil, ErrSessionNotFound
	}
	domain, ok := state.LookupDomain(tenantID, prev.DomainID)
	if !ok {
		return nil, ErrDomainNotFound
	}
	return TriggerSession(state, tenantID, domain, prev.Trigger, newID)
}

// BuildDomainLocation returns the canonical API location for a domain.
func BuildDomainLocation(id string) string {
	return "/api/v1/domains/" + id
}

// BuildSessionLocation returns the canonical API location for a session.
func BuildSessionLocation(id string) string {
	return "/api/v1/sessions/" + id
}

// Error helpers used by HTTP handlers.
var (
	ErrDomainExists    = errors.New("domain already exists")
	ErrDomainNotFound  = errors.New("domain not found")
	ErrSessionExists   = errors.New("session already exists")
	ErrSessionNotFound = errors.New("session not found")
	ErrIDMismatch      = errors.New("domain id mismatch")
)

// NewDomainExistsError formats a domain-exists message.
func NewDomainExistsError(id string) error {
	return fmt.Errorf("domain %s already exists; use PUT to update", id)
}
