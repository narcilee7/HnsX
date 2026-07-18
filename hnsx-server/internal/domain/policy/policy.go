// Package policy defines the Policy aggregate — a stack of rules the
// runtime evaluates against agent actions to decide allow / deny /
// approval_required.
//
// Rules are categorized by Kind so the engine can short-circuit:
//   cost         — runs after each agent Result (token / dollar ceilings)
//   permission   — runs before each tool invocation
//   guardrail    — runs on every observation (regex / jmespath checks)
//   data_egress  — runs on outbound network actions
package policy

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Kind categorizes a rule.
type Kind string

const (
	KindCost        Kind = "cost"
	KindPermission  Kind = "permission"
	KindGuardrail   Kind = "guardrail"
	KindDataEgress  Kind = "data_egress"
)

// Action is what the engine returns when a rule matches.
type Action string

const (
	ActionAllow            Action = "allow"
	ActionDeny             Action = "deny"
	ActionApprovalRequired Action = "approval_required"
)

// Rule is a single policy statement. Expression is JMESPath over the
// EvalContext; the engine evaluates it and returns the Action when it
// matches.
type Rule struct {
	ID         string `json:"id"`
	Kind       Kind   `json:"kind"`
	Expression string `json:"expression"`
	Action     Action `json:"action"`
	Message    string `json:"message,omitempty"`
	Priority   int    `json:"priority"` // lower runs first
}

// Policy is the aggregate root. One Policy can be attached to many
// Harnesses; rules are evaluated in priority order.
type Policy struct {
	ID          string          `gorm:"type:uuid;primaryKey" json:"id"`
	WorkspaceID string          `gorm:"type:uuid;not null;index" json:"workspace_id"`
	Name        string          `gorm:"type:text;not null" json:"name"`
	Description string          `gorm:"type:text;not null;default:''" json:"description"`
	Rules       json.RawMessage `gorm:"type:jsonb;not null;default:'[]'::jsonb;serializer:json" json:"rules"`
	CreatedAt   time.Time       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time       `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Policy) TableName() string { return "policies" }

// Validate enforces invariants.
func (p *Policy) Validate() error {
	if p.WorkspaceID == "" {
		return errors.New("policy: workspace_id is required")
	}
	if p.Name == "" {
		return errors.New("policy: name is required")
	}
	return nil
}

// RulesTyped decodes the rules JSONB column.
func (p *Policy) RulesTyped() ([]Rule, error) {
	if len(p.Rules) == 0 {
		return nil, nil
	}
	var out []Rule
	if err := json.Unmarshal(p.Rules, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// EvalContext is the input to the engine. Implementations live in the
// service layer; this struct is the contract.
type EvalContext struct {
	WorkspaceID string                 `json:"workspace_id"`
	IssueID     string                 `json:"issue_id"`
	AgentID     string                 `json:"agent_id"`
	Action      string                 `json:"action"`     // "tool_call" | "message" | "result"
	ToolName    string                 `json:"tool_name,omitempty"`
	TokensIn    int64                  `json:"tokens_in"`
	TokensOut   int64                  `json:"tokens_out"`
	CostUSD     float64                `json:"cost_usd"`
	Payload     json.RawMessage        `json:"payload,omitempty"`
	Extra       map[string]any         `json:"extra,omitempty"`
}

// Decision is the engine output.
type Decision struct {
	Action    Action
	RuleID    string
	Message   string
}

// Engine is the policy evaluation port.
type Engine interface {
	Evaluate(ctx context.Context, p *Policy, ec EvalContext) (Decision, error)
}

// Repo is the persistence port.
type Repo interface {
	Create(ctx context.Context, p *Policy) error
	Get(ctx context.Context, id string) (*Policy, error)
	ListByWorkspace(ctx context.Context, workspaceID string) ([]*Policy, error)
	Update(ctx context.Context, p *Policy) error
	Delete(ctx context.Context, id string) error
}