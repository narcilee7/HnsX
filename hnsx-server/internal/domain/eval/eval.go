// Package eval defines the EvalSet / EvalCase / EvalRun aggregates —
// the data flywheel's measurement units.
//
// An EvalSet is a versioned bundle of cases tied to a Harness. When an
// Issue closes (or a CLI user asks), the eval runner evaluates the
// agent's recorded Observation stream against the cases and produces
// an EvalRun with per-case scores.
package eval

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ScorerKind discriminates how a case is scored.
type ScorerKind string

const (
	ScorerExact       ScorerKind = "exact"        // input == expected (string match)
	ScorerRegex       ScorerKind = "regex"        // input matches expected regex
	ScorerContains    ScorerKind = "contains"     // input contains expected substring
	ScorerLLMJudge    ScorerKind = "llm_judge"    // delegate scoring to an LLM
	ScorerCustomFunc  ScorerKind = "custom_func"  // user-provided function
)

// Case is a single test case: input the agent should see, expected
// output (interpretation depends on Scorer).
type Case struct {
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input"`
	Expected json.RawMessage `json:"expected,omitempty"`
	Scorer   ScorerKind      `json:"scorer"`
	Weight   float64         `json:"weight"` // 0..1
}

// EvalSet is the aggregate root. One EvalSet can attach to many
// Harnesses; cases are evaluated as a batch.
type EvalSet struct {
	ID          string          `gorm:"type:uuid;primaryKey" json:"id"`
	WorkspaceID string          `gorm:"type:uuid;not null;index" json:"workspace_id"`
	Name        string          `gorm:"type:text;not null" json:"name"`
	Description string          `gorm:"type:text;not null;default:''" json:"description"`
	Cases       json.RawMessage `gorm:"type:jsonb;not null;default:'[]'::jsonb;serializer:json" json:"cases"`
	Version     string          `gorm:"type:text;not null;default:'1.0.0'" json:"version"`
	CreatedAt   time.Time       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time       `gorm:"autoUpdateTime" json:"updated_at"`
}

func (EvalSet) TableName() string { return "eval_sets" }

// Validate enforces invariants.
func (e *EvalSet) Validate() error {
	if e.WorkspaceID == "" {
		return errors.New("eval set: workspace_id is required")
	}
	if e.Name == "" {
		return errors.New("eval set: name is required")
	}
	return nil
}

// CasesTyped decodes the cases JSONB.
func (e *EvalSet) CasesTyped() ([]Case, error) {
	if len(e.Cases) == 0 {
		return nil, nil
	}
	var out []Case
	if err := json.Unmarshal(e.Cases, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// RunStatus tracks the lifecycle of an EvalRun.
type RunStatus string

const (
	RunPending   RunStatus = "pending"
	RunRunning   RunStatus = "running"
	RunCompleted RunStatus = "completed"
	RunFailed    RunStatus = "failed"
)

// CaseResult is the score for a single case within a Run.
type CaseResult struct {
	CaseName  string  `json:"case_name"`
	Score     float64 `json:"score"` // 0..1
	Passed    bool    `json:"passed"`
	Reason    string  `json:"reason,omitempty"`
	Observed  json.RawMessage `json:"observed,omitempty"`
}

// Run is an EvalSet evaluation in flight or completed.
type Run struct {
	ID          string          `gorm:"type:uuid;primaryKey" json:"id"`
	WorkspaceID string          `gorm:"type:uuid;not null;index" json:"workspace_id"`
	EvalSetID   string          `gorm:"type:uuid;not null;index" json:"eval_set_id"`
	IssueID     *string         `gorm:"type:uuid;index" json:"issue_id,omitempty"`
	HarnessID   *string         `gorm:"type:uuid;index" json:"harness_id,omitempty"`
	Status      RunStatus       `gorm:"type:text;not null;default:'pending'" json:"status"`
	TotalScore  float64         `gorm:"not null;default:0" json:"total_score"`
	Results     json.RawMessage `gorm:"type:jsonb;not null;default:'[]'::jsonb;serializer:json" json:"results"`
	StartedAt   time.Time       `gorm:"autoCreateTime" json:"started_at"`
	CompletedAt *time.Time      `gorm:"type:timestamptz" json:"completed_at,omitempty"`
	Error       string          `gorm:"type:text;not null;default:''" json:"error,omitempty"`
}

func (Run) TableName() string { return "eval_runs" }

// Repo is the persistence port for EvalSets.
type EvalSetRepo interface {
	Create(ctx context.Context, e *EvalSet) error
	Get(ctx context.Context, id string) (*EvalSet, error)
	ListByWorkspace(ctx context.Context, workspaceID string) ([]*EvalSet, error)
	Update(ctx context.Context, e *EvalSet) error
	Delete(ctx context.Context, id string) error
}

// RunRepo is the persistence port for Runs.
type RunRepo interface {
	Create(ctx context.Context, r *Run) error
	Get(ctx context.Context, id string) (*Run, error)
	ListByEvalSet(ctx context.Context, evalSetID string, limit int) ([]*Run, error)
	Update(ctx context.Context, r *Run) error
}

// Runner is the eval execution port.
type Runner interface {
	Run(ctx context.Context, run *Run, es *EvalSet, observations []*ObservationRef) error
}

// ObservationRef is the input the runner scores against. We use the
// domain.observation type via its Sink port to keep imports clean.
type ObservationRef struct {
	Kind     string
	Sequence int64
	Payload  json.RawMessage
}