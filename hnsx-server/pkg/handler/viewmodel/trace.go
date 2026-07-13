package viewmodel

import "time"

// TraceListItem is the canonical list view of a trace.
type TraceListItem struct {
	TraceID               string    `json:"trace_id"`
	SessionID             string    `json:"session_id"`
	DomainID              string    `json:"domain_id"`
	DomainVersion         string    `json:"domain_version"`
	Status                string    `json:"status,omitempty"`
	StartedAt             time.Time `json:"started_at,omitempty"`
	CompletedAt           time.Time `json:"completed_at,omitempty"`
	DurationMs            int64     `json:"duration_ms,omitempty"`
	ObservationCount      int       `json:"observation_count,omitempty"`
	TotalCostUSD          float64   `json:"total_cost_usd,omitempty"`
	TotalPromptTokens     int       `json:"prompt_tokens,omitempty"`
	TotalCompletionTokens int       `json:"completion_tokens,omitempty"`
	AgentInvocations      int       `json:"agent_invocations,omitempty"`
	ToolInvocations       int       `json:"tool_invocations,omitempty"`
	CreatedAt             time.Time `json:"created_at,omitempty"`
}

// TraceList is a paginated list of traces.
type TraceList struct {
	Items  []TraceListItem `json:"items"`
	Total  int             `json:"total"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

// TraceDetail is the canonical detail view of a trace.
type TraceDetail struct {
	TraceID               string            `json:"trace_id"`
	SessionID             string            `json:"session_id"`
	DomainID              string            `json:"domain_id"`
	DomainVersion         string            `json:"domain_version"`
	Status                string            `json:"status,omitempty"`
	StartedAt             time.Time         `json:"started_at,omitempty"`
	CompletedAt           time.Time         `json:"completed_at,omitempty"`
	DurationMs            int64             `json:"duration_ms,omitempty"`
	ObservationCount      int               `json:"observation_count,omitempty"`
	TotalCostUSD          float64           `json:"total_cost_usd,omitempty"`
	TotalPromptTokens     int               `json:"prompt_tokens,omitempty"`
	TotalCompletionTokens int               `json:"completion_tokens,omitempty"`
	AgentInvocations      int               `json:"agent_invocations,omitempty"`
	ToolInvocations       int               `json:"tool_invocations,omitempty"`
	Observations          []ObservationItem `json:"observations"`
}

// ObservationItem is a single observation in a trace detail.
type ObservationItem struct {
	TraceID       string         `json:"trace_id,omitempty"`
	SessionID     string         `json:"session_id,omitempty"`
	DomainID      string         `json:"domain_id,omitempty"`
	DomainVersion string         `json:"domain_version,omitempty"`
	StepID        string         `json:"step_id,omitempty"`
	AgentID       string         `json:"agent_id,omitempty"`
	Kind          string         `json:"kind"`
	Payload       map[string]any `json:"payload,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
	CostUSD          float64   `json:"cost_usd,omitempty"`
	PromptTokens     int       `json:"prompt_tokens,omitempty"`
	CompletionTokens int       `json:"completion_tokens,omitempty"`
	LatencyMs        int64     `json:"latency_ms,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}
