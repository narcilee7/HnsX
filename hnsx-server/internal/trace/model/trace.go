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
		CreatedAt: obs.Timestamp,
	}
	if r.Payload == nil {
		r.Payload = map[string]any{}
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

// Common trace errors.
var (
	ErrTraceNotFound = errors.New("trace: not found")
)
