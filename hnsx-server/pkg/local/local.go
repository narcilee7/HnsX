// Package local holds pure, local-only commands that can be shared by the
// CLI and the server without pulling in DB, HTTP, gRPC, or OTel dependencies.
package local

import (
	"encoding/json"
	"io"

	"gopkg.in/yaml.v3"

	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// DomainSummary is the output of ValidateDomain.
type DomainSummary struct {
	Valid      bool
	ID         string
	Version    string
	Mode       spec.HarnessSessionMode
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

// DecodeDomainSpec parses either YAML or JSON into a validated *DomainSpec.
func DecodeDomainSpec(r io.Reader, contentType string) (*spec.DomainSpec, error) {
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

// decodeDomainSpec is kept as an alias for internal use within this package.
func decodeDomainSpec(r io.Reader, contentType string) (*spec.DomainSpec, error) {
	return DecodeDomainSpec(r, contentType)
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
