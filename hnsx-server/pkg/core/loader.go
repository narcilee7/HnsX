package core

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadDomain loads a DomainSpec from a YAML or JSON file.
func LoadDomain(path string) (*DomainSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var spec DomainSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("unmarshal domain spec: %w", err)
	}

	if err := Validate(&spec); err != nil {
		return nil, err
	}

	return &spec, nil
}

// Validate checks a DomainSpec for structural correctness.
func Validate(spec *DomainSpec) error {
	if spec.ID == "" {
		return fmt.Errorf("domain id is required")
	}
	if spec.Version == "" {
		return fmt.Errorf("domain version is required")
	}
	if len(spec.Harness.Agents) == 0 {
		return fmt.Errorf("at least one agent is required")
	}

	agentIDs := make(map[string]bool)
	for _, agent := range spec.Harness.Agents {
		if agent.ID == "" {
			return fmt.Errorf("agent id is required")
		}
		if agentIDs[agent.ID] {
			return fmt.Errorf("duplicate agent id: %s", agent.ID)
		}
		agentIDs[agent.ID] = true
		if agent.Model.Provider == "" {
			return fmt.Errorf("agent %s: model provider is required", agent.ID)
		}
		if agent.Model.Model == "" {
			return fmt.Errorf("agent %s: model name is required", agent.ID)
		}
		if agent.Adapter.Kind == "" {
			return fmt.Errorf("agent %s: adapter kind is required", agent.ID)
		}
	}

	if spec.Harness.Session.Mode == "" {
		spec.Harness.Session.Mode = "single-task"
	}

	if spec.Harness.Session.Mode == "workflow" {
		if spec.Harness.Session.Workflow == nil {
			return fmt.Errorf("workflow mode requires workflow definition")
		}
		if spec.Harness.Session.Workflow.Entry == "" {
			return fmt.Errorf("workflow entry is required")
		}
		stepIDs := make(map[string]bool)
		for _, step := range spec.Harness.Session.Workflow.Steps {
			if step.ID == "" {
				return fmt.Errorf("workflow step id is required")
			}
			if step.AgentRef == "" {
				return fmt.Errorf("workflow step %s: agent is required", step.ID)
			}
			if !agentIDs[step.AgentRef] {
				return fmt.Errorf("workflow step %s: agent %s not found", step.ID, step.AgentRef)
			}
			stepIDs[step.ID] = true
		}
		if !stepIDs[spec.Harness.Session.Workflow.Entry] {
			return fmt.Errorf("workflow entry %s not found in steps", spec.Harness.Session.Workflow.Entry)
		}
	}

	return nil
}

// ToJSON serializes a DomainSpec to JSON.
func ToJSON(spec *DomainSpec) ([]byte, error) {
	return json.MarshalIndent(spec, "", "  ")
}
