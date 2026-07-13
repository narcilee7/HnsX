// Domain decoding helpers — moved from pkg/local/local.go in Phase 3.

package domain

import (
	"encoding/json"
	"io"

	"gopkg.in/yaml.v3"
)

// DomainSummary is the output of ValidateDomain.
type DomainSummary struct {
	Valid      bool
	ID         string
	Version    string
	Mode       HarnessSessionMode
	AgentCount int
	StepCount  int
}

// ValidateDomain parses and validates a DomainSpec from a reader and returns
// a summary. Body can be JSON or YAML.
func ValidateDomain(r io.Reader, contentType string) (*DomainSummary, error) {
	s, err := DecodeDomainSpec(r, contentType)
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

// DecodeDomainSpec parses either YAML or JSON into a validated *DomainSpec.
func DecodeDomainSpec(r io.Reader, contentType string) (*DomainSpec, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var s DomainSpec
	if IsYAMLContentType(contentType) || looksLikeYAML(body) {
		if err := yaml.Unmarshal(body, &s); err != nil {
			return nil, err
		}
	} else {
		if err := json.Unmarshal(body, &s); err != nil {
			return nil, err
		}
	}
	if err := Validate(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// IsYAMLContentType returns true for explicit YAML content types.
func IsYAMLContentType(ct string) bool {
	switch ct {
	case "application/yaml", "application/x-yaml", "text/yaml":
		return true
	default:
		return false
	}
}

// looksLikeYAML is a best-effort sniff for the few cases where clients
// forget to set Content-Type but send YAML anyway (e.g. raw POSTs from
// the CLI's `hnsx domain apply --file`).
func looksLikeYAML(data []byte) bool {
	trimmed := data
	// strip leading whitespace
	for len(trimmed) > 0 && (trimmed[0] == ' ' || trimmed[0] == '\n' || trimmed[0] == '\r' || trimmed[0] == '\t') {
		trimmed = trimmed[1:]
	}
	if len(trimmed) == 0 {
		return false
	}
	// JSON must start with `{` or `[`; YAML typically starts with a key
	// (`foo:`) or a list marker (`-`).
	switch trimmed[0] {
	case '{', '[':
		return false
	}
	return true
}
