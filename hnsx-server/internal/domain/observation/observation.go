// Package observation defines the Observation value object and its Sink port.
//
// Observations are the data flywheel's atomic unit. The daemon emits one
// per agent Message (and per policy decision / eval score); the Sink writes
// them to Postgres with dimension tags that let the EvalSet runner slice
// regressions by prompt_hash / agent_template_id / tool_signatures.
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
	PolicyAllow           PolicyDecision = "allow"
	PolicyDeny            PolicyDecision = "deny"
	PolicyApprovalRequired PolicyDecision = "approval_required"
)

// Observation is the value object written to the sink.
type Observation struct {
	ID         string
	WorkspaceID string
	IssueID    string
	AgentID    string

	Kind     Kind
	Sequence int64
	Payload  json.RawMessage
	OccurredAt time.Time

	// Flywheel dimensions (R3 fills these in)
	PromptHash       string          // sha256 of the rendered prompt
	AgentTemplateID  string          // agent_template_id from the resolved harness
	ToolSignatures   []string        // tool names invoked this turn
	PolicyDecision   PolicyDecision  // for Kind == KindPolicyDecision
	EvalRunID        string          // for Kind == KindEvalScore
}

// Sink is the persistence port. Implemented by infra/db/postgres.
type Sink interface {
	Write(ctx context.Context, obs *Observation) error
	ListByIssue(ctx context.Context, issueID string, limit int) ([]*Observation, error)
	ListByEvalRun(ctx context.Context, evalRunID string) ([]*Observation, error)
}