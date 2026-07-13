// Observation types — moved from pkg/runtime/observation.go in Phase 3.
// See `docs/REFACTOR_PLAN.md` for the migration rationale.

package domain

import "time"

// Observation is the unit emitted by the runtime for downstream consumers.
//
// Kind is the coarse event taxonomy. The legal kinds are documented in
// `docs/know-how/我们如何观测Harness与Agent.md` §3.2.
type Observation struct {
	Kind      string         `json:"kind"`
	SessionID string         `json:"session_id,omitempty"`
	DomainID  string         `json:"domain_id,omitempty"`
	StepID    string         `json:"step_id,omitempty"`
	AgentID   string         `json:"agent_id,omitempty"`
	ParentID  string         `json:"parent_id,omitempty"`
	TraceID   string         `json:"trace_id,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Cost      *Cost          `json:"cost,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// Cost captures per-call spend. Populated when the adapter can report it
// (real adapters); NoopAdapter leaves it zero.
type Cost struct {
	PromptTokens     int     `json:"prompt_tokens,omitempty"`
	CompletionTokens int     `json:"completion_tokens,omitempty"`
	CostUSD          float64 `json:"cost_usd,omitempty"`
	LatencyMs        int64   `json:"latency_ms,omitempty"`
}
