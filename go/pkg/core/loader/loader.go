// Package loader parses domain YAML/JSON and upgrades legacy v1 specs to v2.
package loader

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/hnsx-io/hnsx/go/pkg/core/domain"
	"gopkg.in/yaml.v3"
)

// rawV1Agent mirrors the legacy v1 AgentSpec shape.
type rawV1Agent struct {
	ID          string          `yaml:"id" json:"id"`
	Description string          `yaml:"description" json:"description"`
	Model       rawV1Model      `yaml:"model" json:"model"`
	Adapter     json.RawMessage `yaml:"adapter" json:"adapter"`
	Prompt      rawV1Prompt     `yaml:"prompt" json:"prompt"`
	Sandbox     *domain.SandboxSpec `yaml:"sandbox,omitempty" json:"sandbox,omitempty"`
	MemoryWindow int            `yaml:"memory_window,omitempty" json:"memory_window,omitempty"`
}

type rawV1Model struct {
	Provider string `yaml:"provider" json:"provider"`
	Model    string `yaml:"model" json:"model"`
	Endpoint string `yaml:"endpoint,omitempty" json:"endpoint,omitempty"`
}

type rawV1Prompt struct {
	Template  string          `yaml:"template" json:"template"`
	Variables json.RawMessage `yaml:"variables" json:"variables"`
}

// rawV1Domain mirrors the legacy v1 DomainSpec shape.
type rawV1Domain struct {
	ID          string              `yaml:"id" json:"id"`
	Version     string              `yaml:"version" json:"version"`
	Description string              `yaml:"description" json:"description"`
	Agents      []rawV1Agent        `yaml:"agents" json:"agents"`
	Workflow    domain.WorkflowSpec `yaml:"workflow" json:"workflow"`
	Memory      *domain.MemoryConfig `yaml:"memory,omitempty" json:"memory,omitempty"`
	Sandbox     *domain.SandboxSpec  `yaml:"sandbox,omitempty" json:"sandbox,omitempty"`
}

// LoadDomain reads a domain spec from disk and normalizes it to v2.
func LoadDomain(path string) (*domain.DomainSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read domain file: %w", err)
	}

	// Try v2 first.
	var v2 domain.DomainSpec
	if err := yaml.Unmarshal(data, &v2); err == nil && v2.Harness.Session.Mode != "" {
		if err := validate(&v2); err != nil {
			return nil, err
		}
		return &v2, nil
	}

	// Fall back to v1 upgrade.
	var v1 rawV1Domain
	if err := yaml.Unmarshal(data, &v1); err != nil {
		return nil, fmt.Errorf("parse domain yaml: %w", err)
	}

	spec, err := upgradeV1(&v1)
	if err != nil {
		return nil, fmt.Errorf("upgrade v1 spec: %w", err)
	}

	if err := validate(spec); err != nil {
		return nil, err
	}

	return spec, nil
}

func upgradeV1(v1 *rawV1Domain) (*domain.DomainSpec, error) {
	if len(v1.Agents) == 0 {
		return nil, fmt.Errorf("no agents found in v1 spec")
	}

	agents := make(map[string]domain.AgentSpec, len(v1.Agents))
	prompts := make(map[string]domain.PromptSpec, len(v1.Agents))

	for _, a := range v1.Agents {
		if a.ID == "" {
			return nil, fmt.Errorf("v1 agent missing id")
		}

		adapter := a.Adapter
		if adapter == nil {
			adapter = json.RawMessage("{}")
		}

		// Merge model fields into adapter for provider-specific consumption.
		mergedAdapter := map[string]any{}
		if err := json.Unmarshal(adapter, &mergedAdapter); err != nil {
			return nil, fmt.Errorf("parse adapter for agent %s: %w", a.ID, err)
		}
		if a.Model.Model != "" {
			mergedAdapter["model"] = a.Model.Model
		}
		if a.Model.Endpoint != "" {
			mergedAdapter["endpoint"] = a.Model.Endpoint
		}
		adapterBytes, err := json.Marshal(mergedAdapter)
		if err != nil {
			return nil, err
		}

		promptID := a.ID + "-prompt"
		prompts[promptID] = domain.PromptSpec{
			Type:     "system",
			Template: a.Prompt.Template,
			Schema:   nil,
		}

		agents[a.ID] = domain.AgentSpec{
			ID:           a.ID,
			Provider:     a.Model.Provider,
			Model:        a.Model.Model,
			Adapter:      adapterBytes,
			SystemPrompt: promptID,
		}
	}

	sandbox := domain.SandboxSpec{Policy: "none"}
	if v1.Sandbox != nil {
		sandbox = *v1.Sandbox
	}

	spec := &domain.DomainSpec{
		ID:          v1.ID,
		Version:     v1.Version,
		Description: v1.Description,
		Harness: domain.HarnessSpec{
			Agents:  agents,
			Prompts: prompts,
			Sandbox: sandbox,
			Memory:  v1.Memory,
			Session: domain.SessionSpec{
				Mode:     "workflow",
				Workflow: &v1.Workflow,
			},
		},
	}

	return spec, nil
}

func validate(spec *domain.DomainSpec) error {
	if spec.ID == "" {
		return fmt.Errorf("domain id is required")
	}
	if spec.Version == "" {
		return fmt.Errorf("domain version is required")
	}
	if len(spec.Harness.Agents) == 0 {
		return fmt.Errorf("harness.agents cannot be empty")
	}

	switch spec.Harness.Session.Mode {
	case "single-task", "multi-turn", "hierarchical", "autonomous", "workflow":
		// ok
	default:
		return fmt.Errorf("unknown session mode: %s", spec.Harness.Session.Mode)
	}

	if spec.Harness.Session.Mode == "workflow" && spec.Harness.Session.Workflow == nil {
		return fmt.Errorf("session mode workflow requires harness.session.workflow")
	}

	if spec.Harness.Session.Agent != "" {
		if _, ok := spec.Harness.Agents[spec.Harness.Session.Agent]; !ok {
			return fmt.Errorf("session references unknown agent: %s", spec.Harness.Session.Agent)
		}
	}

	if spec.Harness.Session.Workflow != nil {
		entryFound := false
		for _, s := range spec.Harness.Session.Workflow.Steps {
			if _, ok := spec.Harness.Agents[s.Agent]; !ok {
				return fmt.Errorf("workflow step '%s' references unknown agent '%s'", s.ID, s.Agent)
			}
			if s.ID == spec.Harness.Session.Workflow.Entry {
				entryFound = true
			}
		}
		if !entryFound {
			return fmt.Errorf("workflow.entry '%s' not found in steps", spec.Harness.Session.Workflow.Entry)
		}
	}

	for id, a := range spec.Harness.Agents {
		if a.Provider == "" {
			return fmt.Errorf("agent '%s' provider is required", id)
		}
	}

	return nil
}
