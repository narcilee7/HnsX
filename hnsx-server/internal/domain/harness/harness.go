// Package harness defines the Harness aggregate — the bundle that ties
// an Agent to its prompts, skills, tools, policy, and EvalSet.
//
// A Harness is the unit of evaluation and reproducibility: when an
// issue is assigned to an agent, the harness pinned to that agent
// (or the workspace default) determines exactly what runs and what
// gets measured. The Observation's prompt_hash / agent_template_id /
// tool_signatures dimensions are derived from the harness so the data
// flywheel can slice regressions by version.
package harness

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Prompt is a named template that the agent runtime renders before
// each turn. Vars are extracted from the issue description or workspace
// context at execution time.
type Prompt struct {
	Name     string   `json:"name"`
	Template string   `json:"template"`
	Vars     []string `json:"vars,omitempty"`
}

// SkillRef pins a skill (by ID + version) into the harness. Locking
// the version lets eval runs be reproducible across agent upgrades.
type SkillRef struct {
	SkillID string `json:"skill_id"`
	Version string `json:"version"`
}

// ToolRef describes a tool the agent can invoke. Signature is used as a
// flywheel dimension (ToolSignatures on Observation).
type ToolRef struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
}

// Harness is the aggregate root. One Harness per (Workspace, Name).
type Harness struct {
	ID            string          `gorm:"type:uuid;primaryKey" json:"id"`
	WorkspaceID   string          `gorm:"type:uuid;not null;index" json:"workspace_id"`
	Name          string          `gorm:"type:text;not null" json:"name"`
	Description   string          `gorm:"type:text;not null;default:''" json:"description"`
	Prompts       json.RawMessage `gorm:"type:jsonb;not null;default:'[]'::jsonb;serializer:json" json:"prompts"`
	Skills        json.RawMessage `gorm:"type:jsonb;not null;default:'[]'::jsonb;serializer:json" json:"skills"`
	Tools         json.RawMessage `gorm:"type:jsonb;not null;default:'[]'::jsonb;serializer:json" json:"tools"`
	PolicyID      *string         `gorm:"type:uuid;index" json:"policy_id,omitempty"`
	EvalSetID     *string         `gorm:"type:uuid;index" json:"eval_set_id,omitempty"`
	Version       string          `gorm:"type:text;not null;default:'1.0.0'" json:"version"`
	CreatedAt     time.Time       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time       `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Harness) TableName() string { return "harness_bindings" }

// Validate enforces invariants.
func (h *Harness) Validate() error {
	if h.WorkspaceID == "" {
		return errors.New("harness: workspace_id is required")
	}
	if h.Name == "" {
		return errors.New("harness: name is required")
	}
	return nil
}

// PromptsTyped returns the prompts slice decoded, with a default empty
// slice if the column was never populated.
func (h *Harness) PromptsTyped() ([]Prompt, error) {
	if len(h.Prompts) == 0 {
		return nil, nil
	}
	var out []Prompt
	if err := json.Unmarshal(h.Prompts, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Repo is the persistence port.
type Repo interface {
	Create(ctx context.Context, h *Harness) error
	Get(ctx context.Context, id string) (*Harness, error)
	ListByWorkspace(ctx context.Context, workspaceID string) ([]*Harness, error)
	Update(ctx context.Context, h *Harness) error
	Delete(ctx context.Context, id string) error
}