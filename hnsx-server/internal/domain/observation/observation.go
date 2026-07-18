// Package observation defines the Observation value object and its Sink port.
//
// Observations are the data flywheel's atomic unit. The daemon emits one
// per agent Message (and per policy decision / eval score); the Sink writes
// them to Postgres with dimension tags that let the EvalSet runner slice
// regressions by prompt_hash / agent_template_id / tool_signatures.
//
// Persistence: the struct doubles as the GORM model.
package observation

import (
	"context"
	"encoding/json"
	"time"
)

// Kind discriminates observation types.
type Kind string

const (
	KindMessage         Kind = "message"          // agent message (assistant / tool_use / tool_result)
	KindRoutingDecision Kind = "routing_decision" // squad leader dispatch choice
	KindPolicyDecision  Kind = "policy_decision"  // policy rule evaluated
	KindApprovalEvent   Kind = "approval_event"   // approval requested/granted/denied
	KindEvalScore       Kind = "eval_score"       // scorer output after issue close
)

// PolicyDecision captures the outcome of a policy evaluation.
type PolicyDecision string

const (
	PolicyAllow            PolicyDecision = "allow"
	PolicyDeny             PolicyDecision = "deny"
	PolicyApprovalRequired PolicyDecision = "approval_required"
)

// Observation is the value object written to the sink.
type Observation struct {
	ID         string          `gorm:"type:uuid;primaryKey" json:"id"`
	WorkspaceID string         `gorm:"type:uuid;not null" json:"workspace_id"`
	IssueID    string          `gorm:"type:uuid;not null;index" json:"issue_id"`
	AgentID    string          `gorm:"type:uuid;not null" json:"agent_id"`

	Kind     Kind            `gorm:"type:text;not null" json:"kind"`
	Sequence int64           `gorm:"not null;default:0" json:"sequence"`
	Payload  json.RawMessage `gorm:"type:jsonb;not null;default:'{}'::jsonb" json:"payload"`
	OccurredAt time.Time     `gorm:"autoCreateTime;index" json:"occurred_at"`

	// Flywheel dimensions (R3 fills these in)
	PromptHash       string          `gorm:"type:text;not null;default:''" json:"prompt_hash"`
	AgentTemplateID  string          `gorm:"type:text;not null;default:''" json:"agent_template_id"`
	ToolSignatures   json.RawMessage `gorm:"type:jsonb;not null;default:'[]'::jsonb" json:"tool_signatures"`
	PolicyDecision   PolicyDecision  `gorm:"type:text;not null;default:''" json:"policy_decision"`
	EvalRunID        string          `gorm:"type:text;not null;default:'';index" json:"eval_run_id"`
}

func (Observation) TableName() string { return "observations" }

// Sink is the persistence port. Implemented by infra/db/postgres.
type Sink interface {
	Write(ctx context.Context, obs *Observation) error
	ListByIssue(ctx context.Context, issueID string, limit int) ([]*Observation, error)
	ListByEvalRun(ctx context.Context, evalRunID string) ([]*Observation, error)
}