package viewmodel

import "time"

// SessionListItem is the canonical list view of a session.
type SessionListItem struct {
	ID            string     `json:"id"`
	DomainID      string     `json:"domain_id"`
	DomainVersion string     `json:"domain_version"`
	Orchestration string     `json:"orchestration"`
	State         string     `json:"state"`
	StartedAt     time.Time  `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	Summary       *SessionSummary `json:"summary,omitempty"`
}

// SessionList is a paginated list of sessions.
type SessionList struct {
	Items  []SessionListItem `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

// SessionDetail is the canonical detail view of a session.
type SessionDetail struct {
	ID            string         `json:"id"`
	DomainID      string         `json:"domain_id"`
	DomainVersion string         `json:"domain_version"`
	Orchestration string         `json:"orchestration"`
	State         string         `json:"state"`
	Trigger       map[string]any `json:"trigger,omitempty"`
	StartedAt     time.Time      `json:"started_at"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
	Result        map[string]any `json:"result,omitempty"`
	Summary       *SessionSummary `json:"summary,omitempty"`
}

// SessionSummary contains aggregated runtime statistics for a session.
type SessionSummary struct {
	DurationMs       uint64  `json:"duration_ms"`
	Mode             string  `json:"mode"`
	AgentInvocations int     `json:"agent_invocations"`
	ToolInvocations  int     `json:"tool_invocations"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
}

// SessionTriggered is returned after a successful trigger/rerun.
type SessionTriggered struct {
	ID    string `json:"id"`
	State string `json:"state"`
}

// SessionFilters are supported server-side filters for session lists.
type SessionFilters struct {
	DomainID string
	State    string
}
