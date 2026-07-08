// Package telemetry provides trace, metric, and audit collection.
package telemetry

import (
	"context"
	"time"
)

// Sink receives telemetry records.
type Sink interface {
	RecordTrace(ctx context.Context, record *TraceRecord) error
	RecordInvocation(ctx context.Context, record *InvocationRecord) error
	RecordAudit(ctx context.Context, record *AuditRecord) error
}

// TraceRecord is an observation trace.
type TraceRecord struct {
	TraceID   string
	SessionID string
	DomainID  string
	StepID    string
	AgentID   string
	Kind      string
	Payload   string
	CreatedAt time.Time
}

// InvocationRecord captures cost and latency.
type InvocationRecord struct {
	SessionID        string
	DomainID         string
	StartedAt        time.Time
	Duration         time.Duration
	TotalCostUSD     float64
	PromptTokens     int64
	CompletionTokens int64
}

// AuditRecord is an immutable audit log entry.
type AuditRecord struct {
	RecordID  string
	SessionID string
	DomainID  string
	Action    string
	Actor     string
	Details   string
	CreatedAt time.Time
}

// StdoutSink writes telemetry to stdout.
type StdoutSink struct{}

// NewStdoutSink creates a stdout sink.
func NewStdoutSink() *StdoutSink {
	return &StdoutSink{}
}

func (s *StdoutSink) RecordTrace(ctx context.Context, record *TraceRecord) error {
	return nil
}

func (s *StdoutSink) RecordInvocation(ctx context.Context, record *InvocationRecord) error {
	return nil
}

func (s *StdoutSink) RecordAudit(ctx context.Context, record *AuditRecord) error {
	return nil
}
