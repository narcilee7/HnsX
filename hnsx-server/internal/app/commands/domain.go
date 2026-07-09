// Package commands implements application use cases that are shared between
// the CLI and the HTTP API.
package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// DomainSummary is the output of ValidateDomain.
type DomainSummary struct {
	Valid      bool
	ID         string
	Version    string
	Mode       string
	AgentCount int
	StepCount  int
}

// ValidateDomain parses and validates a DomainSpec from a reader and returns
// a summary. Body can be JSON or YAML.
func ValidateDomain(r io.Reader, contentType string) (*DomainSummary, error) {
	s, err := decodeDomainSpec(r, contentType)
	if err != nil {
		return nil, err
	}

	count := len(s.Harness.Agents)
	steps := 0
	if s.Harness.Session.Workflow != nil {
		steps = len(s.Harness.Session.Workflow.Steps)
	}
	return &DomainSummary{
		Valid:      true,
		ID:         s.ID,
		Version:    s.Version,
		Mode:       s.Harness.Session.Mode,
		AgentCount: count,
		StepCount:  steps,
	}, nil
}

// RegisterResult is returned by RegisterDomain.
type RegisterResult struct {
	Domain    *app.RegisteredDomain
	CreatedAt time.Time
}

// RegisterDomain parses a DomainSpec from a reader and registers it in the
// application state. Returns ErrDomainExists if the ID is already registered.
func RegisterDomain(state *app.State, r io.Reader, contentType string) (*RegisterResult, error) {
	if state == nil {
		return nil, errors.New("nil app state")
	}
	s, err := decodeDomainSpec(r, contentType)
	if err != nil {
		return nil, err
	}

	if _, exists := state.LookupDomain(s.ID); exists {
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
	state.RegisterDomain(d)
	return &RegisterResult{Domain: d, CreatedAt: now}, nil
}

// UpdateDomain replaces the fields of an existing registered domain with the
// parsed body. Returns ErrDomainNotFound / ErrIDMismatch as appropriate.
func UpdateDomain(state *app.State, id string, r io.Reader, contentType string) (*app.RegisteredDomain, error) {
	if state == nil {
		return nil, errors.New("nil app state")
	}
	existing, ok := state.LookupDomain(id)
	if !ok {
		return nil, ErrDomainNotFound
	}

	s, err := decodeDomainSpec(r, contentType)
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
	state.RegisterDomain(existing) // re-register to refresh map
	return existing, nil
}

// DeleteDomain removes a domain from the application state.
func DeleteDomain(state *app.State, id string) error {
	if state == nil {
		return errors.New("nil app state")
	}
	if _, ok := state.LookupDomain(id); !ok {
		return ErrDomainNotFound
	}
	state.DeleteDomain(id)
	return nil
}

// decodeDomainSpec parses either YAML or JSON into a validated *DomainSpec.
func decodeDomainSpec(r io.Reader, contentType string) (*spec.DomainSpec, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var s spec.DomainSpec
	if isYAMLContentType(contentType) || looksLikeYAML(body) {
		if err := yaml.Unmarshal(body, &s); err != nil {
			return nil, err
		}
	} else {
		if err := json.Unmarshal(body, &s); err != nil {
			return nil, err
		}
	}
	if err := spec.Validate(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// isYAMLContentType returns true for explicit YAML content types.
func isYAMLContentType(ct string) bool {
	switch ct {
	case "application/yaml", "application/x-yaml", "text/yaml":
		return true
	default:
		return false
	}
}

// looksLikeYAML heuristically detects YAML bodies (e.g. leading "---").
func looksLikeYAML(data []byte) bool {
	for i := 0; i < len(data); i++ {
		if data[i] == ' ' || data[i] == '\t' || data[i] == '\n' || data[i] == '\r' {
			continue
		}
		return data[i] == '-'
	}
	return false
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
func TriggerSession(state *app.State, domain *app.RegisteredDomain, trigger map[string]any, newID func(string) string) (*app.RegisteredSession, error) {
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
	state.RegisterSession(sess)
	return sess, nil
}

// CancelSession transitions a non-terminal session to cancelled.
func CancelSession(state *app.State, id string) (*app.RegisteredSession, error) {
	if state == nil {
		return nil, errors.New("nil app state")
	}
	sess, ok := state.LookupSession(id)
	if !ok {
		return nil, ErrSessionNotFound
	}
	if sess.State == "completed" || sess.State == "failed" {
		return nil, fmt.Errorf("session is already in terminal state %q", sess.State)
	}
	state.UpdateSessionState(id, "canceled")
	state.DetachBroadcaster(id)
	return sess, nil
}

// RerunSession creates a new session reusing the trigger of an existing one.
func RerunSession(state *app.State, prevID string, newID func(string) string) (*app.RegisteredSession, error) {
	if state == nil {
		return nil, errors.New("nil app state")
	}
	prev, ok := state.LookupSession(prevID)
	if !ok {
		return nil, ErrSessionNotFound
	}
	domain, ok := state.LookupDomain(prev.DomainID)
	if !ok {
		return nil, ErrDomainNotFound
	}
	return TriggerSession(state, domain, prev.Trigger, newID)
}

// BuildDomainLocation returns the canonical API location for a domain.
func BuildDomainLocation(id string) string {
	return "/api/v1/domains/" + id
}

// BuildSessionLocation returns the canonical API location for a session.
func BuildSessionLocation(id string) string {
	return "/api/v1/sessions/" + id
}

// Error helpers used by HTTP handlers; kept here so CLI can reuse messages.
var (
	ErrDomainExists   = errors.New("domain already exists")
	ErrDomainNotFound = errors.New("domain not found")
	ErrSessionExists  = errors.New("session already exists")
	ErrSessionNotFound = errors.New("session not found")
	ErrIDMismatch     = errors.New("domain id mismatch")
)

// NewDomainExistsError formats a domain-exists message.
func NewDomainExistsError(id string) error {
	return fmt.Errorf("domain %s already exists; use PUT to update", id)
}
