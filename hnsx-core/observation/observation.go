// Package observation defines the canonical Observation type shared between
// the runtime, the telemetry sinks, the SSE broadcaster, and the API layer.
//
// The exact same struct is serialized to stdout/JSON, persisted into the
// `observations` table, converted into OTel spans, and pushed to the
// browser via Server-Sent Events.
//
// Defining it in its own package avoids an import cycle between the runtime
// (which produces observations) and the telemetry sinks (which consume
// them).
package observation

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
