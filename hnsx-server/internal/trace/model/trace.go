// Package model defines the Trace aggregate for the HnsX control plane.
package model

import (
	"errors"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

// ObservationRecord is a persisted copy of a runtime.Observation.
type ObservationRecord struct {
	ID               int64
	TraceID          string
	SessionID        string
	DomainID         string
	DomainVersion    string
	StepID           string
	AgentID          string
	Kind             string
	Payload          map[string]any
	Metadata         map[string]any
	CostUSD          float64
	PromptTokens     int
	CompletionTokens int
	LatencyMs        int64
	CreatedAt        time.Time
}

// FromRuntime converts a runtime.Observation into a record.
func FromRuntime(obs runtime.Observation) ObservationRecord {
	r := ObservationRecord{
		TraceID:   obs.TraceID,
		SessionID: obs.SessionID,
		DomainID:  obs.DomainID,
		StepID:    obs.StepID,
		AgentID:   obs.AgentID,
		Kind:      obs.Kind,
		Payload:   obs.Payload,
		Metadata:  obs.Metadata,
		CreatedAt: obs.Timestamp,
	}
	if r.TraceID == "" && r.SessionID != "" {
		// Current convention: one trace per session. When the runtime omits
		// trace_id (local executor, tests), fall back to session_id so the
		// trace list API can group observations correctly.
		r.TraceID = r.SessionID
	}
	if r.Payload == nil {
		r.Payload = map[string]any{}
	}
	if r.Metadata == nil {
		r.Metadata = map[string]any{}
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	if obs.Cost != nil {
		r.CostUSD = obs.Cost.CostUSD
		r.PromptTokens = obs.Cost.PromptTokens
		r.CompletionTokens = obs.Cost.CompletionTokens
		r.LatencyMs = obs.Cost.LatencyMs
	}
	return r
}

// Aggregate is a rolled-up view of observations across one or more sessions.
type Aggregate struct {
	TotalCostUSD          float64
	TotalPromptTokens     int
	TotalCompletionTokens int
	AgentInvocations      int
	ToolInvocations       int
}

type AggregateWithSession struct {
	Aggregate
	SessionID string
}

// TraceSummary is the per-trace rollup that backs GET /api/v1/traces.
// TraceID is the authoritative identifier; in the current worker convention
// it equals SessionID 1:1, but the data model keeps the two distinct so
// multi-session traces (e.g. supervisor→specialist) can carry a single
// trace_id across sessions in the future without an API break.
type TraceSummary struct {
	TraceID              string
	SessionID            string
	DomainID             string
	DomainVersion        string
	StartedAt            time.Time
	CompletedAt          time.Time
	DurationMs           int64
	Status               string
	ObservationCount     int
	TotalCostUSD         float64
	TotalPromptTokens    int
	TotalCompletionTokens int
	AgentInvocations     int
	ToolInvocations      int
}

// TraceDetail is the per-trace detail payload behind GET /api/v1/traces/:id.
// Observations are returned in chronological order.
type TraceDetail struct {
	TraceSummary
	Observations []ObservationRecord
}

// TraceListFilter captures the query parameters used by ListSummaries.
// TenantID is reserved for multi-tenant; current code sets it from the
// gin middleware default (`default`). Zero values for From/To are no-ops.
type TraceListFilter struct {
	TenantID  string
	DomainID  string
	SessionID string
	AgentID   string
	From      time.Time
	To        time.Time
	Limit     int
	Offset    int
}

// TraceSummaryWithCount pairs a TraceSummary with the total number of
// matching rows for pagination.
type TraceSummaryWithCount struct {
	Summaries []TraceSummary
	Total     int
}

// Common trace errors.
var (
	ErrTraceNotFound = errors.New("trace: not found")
)
