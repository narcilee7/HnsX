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

	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// RegisteredDomain is the runtime view of a loaded DomainSpec.
type RegisteredDomain struct {
	ID          string
	Version     string
	Description string
	Spec        *spec.DomainSpec
	Harness     any
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

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
	Domain    *RegisteredDomain
	CreatedAt time.Time
}

// RegisterDomain parses a DomainSpec from a reader and wraps it in a
// RegisteredDomain. Body can be JSON or YAML. Existence checks are left to
// the caller (HTTP handler / CLI command).
func RegisterDomain(r io.Reader, contentType string) (*RegisterResult, error) {
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

	now := time.Now().UTC()
	d := &RegisteredDomain{
		ID:          s.ID,
		Version:     s.Version,
		Description: s.Description,
		Spec:        &s,
		Harness:     s.Harness,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return &RegisterResult{Domain: d, CreatedAt: now}, nil
}

// UpdateDomain replaces the fields of an existing registered domain with the
// parsed body. The caller is responsible for ensuring the URL id matches the
// body id.
func UpdateDomain(existing *RegisteredDomain, r io.Reader, contentType string) (*RegisteredDomain, error) {
	if existing == nil {
		return nil, errors.New("nil existing domain")
	}
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

	existing.Version = s.Version
	existing.Description = s.Description
	existing.Spec = &s
	existing.Harness = s.Harness
	existing.UpdatedAt = time.Now().UTC()
	return existing, nil
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
func TriggerSession(domain *RegisteredDomain, trigger map[string]any, newID func(string) string) (*RegisteredSession, error) {
	if domain == nil || domain.Spec == nil {
		return nil, errors.New("nil domain")
	}
	id := newID(domain.ID)
	return &RegisteredSession{
		ID:            id,
		DomainID:      domain.ID,
		DomainVersion: domain.Version,
		Orchestration: domain.Spec.Harness.Session.Mode,
		State:         "pending",
		Trigger:       trigger,
		StartedAt:     time.Now().UTC(),
	}, nil
}

// RegisteredSession is the runtime metadata for one Session run.
type RegisteredSession struct {
	ID            string
	DomainID      string
	DomainVersion string
	Orchestration string
	State         string
	Trigger       map[string]any
	Result        any
	StartedAt     time.Time
	CompletedAt   *time.Time
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
	ErrIDMismatch     = errors.New("domain id mismatch")
)

// NewDomainExistsError formats a domain-exists message.
func NewDomainExistsError(id string) error {
	return fmt.Errorf("domain %s already exists; use PUT to update", id)
}
