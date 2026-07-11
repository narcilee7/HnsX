package hnsx

// DomainSummary is the list view of a registered domain.
type DomainSummary struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// Domain is the detail view of a registered domain.
type Domain struct {
	ID          string         `json:"id"`
	Version     string         `json:"version"`
	Description string         `json:"description,omitempty"`
	Harness     map[string]any `json:"harness,omitempty"`
	Status      string         `json:"status,omitempty"`
	CreatedAt   string         `json:"created_at,omitempty"`
	UpdatedAt   string         `json:"updated_at,omitempty"`
}

// SessionSummary is the list view of a session.
type SessionSummary struct {
	ID            string `json:"id"`
	DomainID      string `json:"domain_id"`
	DomainVersion string `json:"domain_version,omitempty"`
	Orchestration string `json:"orchestration,omitempty"`
	State         string `json:"state"`
	StartedAt     string `json:"started_at,omitempty"`
	CompletedAt   string `json:"completed_at,omitempty"`
}

// Session is the detail view of a session.
type Session struct {
	ID            string         `json:"id"`
	DomainID      string         `json:"domain_id"`
	DomainVersion string         `json:"domain_version,omitempty"`
	Orchestration string         `json:"orchestration,omitempty"`
	State         string         `json:"state"`
	Trigger       map[string]any `json:"trigger,omitempty"`
	StartedAt     string         `json:"started_at,omitempty"`
	CompletedAt   string         `json:"completed_at,omitempty"`
	Result        map[string]any `json:"result,omitempty"`
}

// TraceSummary is the list view of a trace.
type TraceSummary struct {
	TraceID            string  `json:"trace_id"`
	SessionID          string  `json:"session_id"`
	DomainID           string  `json:"domain_id"`
	DomainVersion      string  `json:"domain_version,omitempty"`
	Status             string  `json:"status"`
	StartedAt          string  `json:"started_at,omitempty"`
	CompletedAt        string  `json:"completed_at,omitempty"`
	DurationMs         int64   `json:"duration_ms,omitempty"`
	ObservationCount   int64   `json:"observation_count,omitempty"`
	TotalCostUSD       float64 `json:"total_cost_usd,omitempty"`
	PromptTokens       int64   `json:"prompt_tokens,omitempty"`
	CompletionTokens   int64   `json:"completion_tokens,omitempty"`
	AgentInvocations   int64   `json:"agent_invocations,omitempty"`
	ToolInvocations    int64   `json:"tool_invocations,omitempty"`
}

// Trace is the full trace detail including observations.
type Trace struct {
	TraceSummary
	Observations []Observation `json:"observations,omitempty"`
}

// Observation is one event inside a trace.
type Observation struct {
	Kind             string         `json:"kind"`
	TraceID          string         `json:"trace_id,omitempty"`
	SessionID        string         `json:"session_id,omitempty"`
	DomainID         string         `json:"domain_id,omitempty"`
	DomainVersion    string         `json:"domain_version,omitempty"`
	StepID           string         `json:"step_id,omitempty"`
	AgentID          string         `json:"agent_id,omitempty"`
	Payload          map[string]any `json:"payload,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	CostUSD          float64        `json:"cost_usd,omitempty"`
	PromptTokens     int64          `json:"prompt_tokens,omitempty"`
	CompletionTokens int64          `json:"completion_tokens,omitempty"`
	LatencyMs        int64          `json:"latency_ms,omitempty"`
	Timestamp        string         `json:"timestamp,omitempty"`
}

// Approval represents a human-in-the-loop gate.
type Approval struct {
	ID          string         `json:"id"`
	SessionID   string         `json:"session_id,omitempty"`
	DomainID    string         `json:"domain_id,omitempty"`
	Action      string         `json:"action,omitempty"`
	Resource    string         `json:"resource,omitempty"`
	RiskLevel   string         `json:"risk_level,omitempty"`
	Context     map[string]any `json:"context,omitempty"`
	Status      string         `json:"status"`
	RequestedBy string         `json:"requested_by,omitempty"`
	ReviewedBy  string         `json:"reviewed_by,omitempty"`
	Comment     string         `json:"comment,omitempty"`
	CreatedAt   string         `json:"created_at,omitempty"`
	UpdatedAt   string         `json:"updated_at,omitempty"`
	ResolvedAt  string         `json:"resolved_at,omitempty"`
}

// EvalSetSummary is the list view of an eval set.
type EvalSetSummary struct {
	ID          string `json:"id"`
	SetID       string `json:"set_id"`
	DomainID    string `json:"domain_id"`
	Description string `json:"description,omitempty"`
	CaseCount   int    `json:"case_count,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

// EvalCase is one test case.
type EvalCase struct {
	ID     string         `json:"id"`
	Name   string         `json:"name,omitempty"`
	Input  map[string]any `json:"input"`
	Expect map[string]any `json:"expect,omitempty"`
	Scorer EvalScorer     `json:"scorer,omitempty"`
}

// EvalScorer defines how to score a case.
type EvalScorer struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config,omitempty"`
}

// EvalSet is the detail view of an eval set.
type EvalSet struct {
	EvalSetSummary
	Cases     []EvalCase `json:"cases"`
	UpdatedAt string     `json:"updated_at,omitempty"`
}

// EvalCaseResult is the outcome of one eval case.
type EvalCaseResult struct {
	CaseID     string         `json:"case_id"`
	SessionID  string         `json:"session_id,omitempty"`
	Score      float64        `json:"score,omitempty"`
	Passed     bool           `json:"passed,omitempty"`
	Actual     map[string]any `json:"actual,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
	DurationMs int64          `json:"duration_ms,omitempty"`
	CostUSD    float64        `json:"cost_usd,omitempty"`
}

// EvalRun is the result of running an eval set.
type EvalRun struct {
	ID            string           `json:"id"`
	EvalSetID     string           `json:"eval_set_id"`
	DomainID      string           `json:"domain_id"`
	DomainVersion string           `json:"domain_version,omitempty"`
	Orchestration string           `json:"orchestration,omitempty"`
	State         string           `json:"state"`
	Score         float64          `json:"score,omitempty"`
	TotalCases    int              `json:"total_cases"`
	PassedCases   int              `json:"passed_cases,omitempty"`
	TotalCostUSD  float64          `json:"total_cost_usd,omitempty"`
	DurationMs    int64            `json:"duration_ms,omitempty"`
	Cases         []EvalCaseResult `json:"cases,omitempty"`
	CreatedAt     string           `json:"created_at,omitempty"`
	CompletedAt   string           `json:"completed_at,omitempty"`
}

// ListEnvelope is the standard list response shape.
type ListEnvelope[T any] struct {
	Items  []T `json:"items"`
	Total int `json:"total"`
	Limit int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}
