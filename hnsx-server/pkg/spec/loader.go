// Package spec parses DomainSpec v2 YAML/JSON documents and validates them.
//
// This package is intentionally free of infrastructure dependencies so it can
// be imported by both the CLI and the server.
package spec

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadFile reads a DomainSpec v2 YAML/JSON document from disk and returns a
// validated in-memory model.
func LoadFile(path string) (*DomainSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read domain file: %w", err)
	}
	return Parse(data)
}

// Parse decodes and validates DomainSpec v2 bytes.
func Parse(data []byte) (*DomainSpec, error) {
	spec := new(DomainSpec)
	if err := yaml.Unmarshal(data, spec); err != nil {
		return nil, fmt.Errorf("parse domain yaml: %w", err)
	}
	if err := Validate(spec); err != nil {
		return nil, err
	}
	return spec, nil
}

// MustParse is like Parse but panics on error. Intended for tests.
func MustParse(data []byte) *DomainSpec {
	spec, err := Parse(data)
	if err != nil {
		panic(err)
	}
	return spec
}

// Validate enforces structural invariants on a DomainSpec v2.
func Validate(spec *DomainSpec) error {
	if spec == nil {
		return errors.New("domain spec is nil")
	}
	if spec.ID == "" {
		return errors.New("domain.id is required")
	}
	if spec.Version == "" {
		return errors.New("domain.version is required")
	}
	if len(spec.Harness.Agents) == 0 {
		return errors.New("harness.agents cannot be empty")
	}

	// Session mode must be one of the supported orchestration modes.
	switch spec.Harness.Session.Mode {
	case "single-task", "single", "multi-turn", "supervisor",
		"hierarchical", "autonomous", "workflow":
		// ok
	default:
		return fmt.Errorf("unknown session mode: %q", spec.Harness.Session.Mode)
	}

	// If a single/primary agent is named, it must exist.
	if spec.Harness.Session.Agent != "" {
		if _, ok := spec.Harness.Agents[spec.Harness.Session.Agent]; !ok {
			return fmt.Errorf("session.agent %q does not match any known agent",
				spec.Harness.Session.Agent)
		}
	}

	// Workflow mode requires a workflow definition with a valid entry.
	if spec.Harness.Session.Mode == "workflow" {
		wf := spec.Harness.Session.Workflow
		if wf == nil {
			return errors.New("session.mode=workflow requires session.workflow")
		}
		if wf.Entry == "" {
			return errors.New("session.workflow.entry is required")
		}
		entryFound := false
		for _, s := range wf.Steps {
			if s.ID == "" {
				return errors.New("workflow steps must have id")
			}
			if s.Agent == "" {
				return fmt.Errorf("workflow step %q: agent is required", s.ID)
			}
			if _, ok := spec.Harness.Agents[s.Agent]; !ok {
				return fmt.Errorf("workflow step %q references unknown agent %q",
					s.ID, s.Agent)
			}
			if s.ID == wf.Entry {
				entryFound = true
			}
		}
		if !entryFound {
			return fmt.Errorf("workflow.entry %q not found in steps", wf.Entry)
		}
	}

	// Every agent must declare a provider; adapter is optional (defaults later).
	for id, a := range spec.Harness.Agents {
		if a.Provider == "" {
			return fmt.Errorf("agent %q: provider is required", id)
		}
		if a.ID == "" {
			return fmt.Errorf("agent map contains an agent with empty id")
		}
	}

	// Skill tool references must resolve to configured tools or skill names.
	for id, s := range spec.Harness.Skills {
		if id == "" {
			return errors.New("skill map contains an entry with empty id")
		}
		if s.Prompt == "" {
			return fmt.Errorf("skill %q: prompt is required", id)
		}
	}

	return nil
}
