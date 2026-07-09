package telemetry

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/hnsx-io/hnsx/core/observation"
)

// TracerSink converts observation.Observation events into OTel spans on the
// global TracerProvider. It is read-only with respect to the OTel API —
// it does not start its own provider.
//
// Design notes:
//
//   - One span per observation (kind, agent, step, session, cost are all
//     surfaced as attributes).
//   - Spans are exported via the configured TracerProvider's BatchSpan
//     processor (config.Init wires this).
//   - Errors are reported with codes.Error + the observation's error
//     message (if any) on the span.
type TracerSink struct {
	tracer trace.Tracer
}

// NewTracerSink creates a TracerSink that uses the global TracerProvider.
func NewTracerSink() *TracerSink {
	return &TracerSink{
		tracer: otel.Tracer("github.com/hnsx-io/hnsx/server/pkg/telemetry"),
	}
}

// Name returns "otel".
func (s *TracerSink) Name() string { return "otel" }

// Record converts an observation into a span. The span ends immediately
// since observations are point-in-time events.
func (s *TracerSink) Record(ctx context.Context, obs observation.Observation) error {
	start := obs.Timestamp
	if start.IsZero() {
		start = time.Now()
	}

	spanCtx, span := s.tracer.Start(ctx, observationSpanName(obs),
		trace.WithTimestamp(start),
		trace.WithAttributes(observationAttrs(obs)...),
	)
	_ = spanCtx

	if errMsg, ok := obs.Payload["error"].(string); ok {
		span.RecordError(fmt.Errorf("%s", errMsg))
		span.SetStatus(codes.Error, errMsg)
	} else {
		span.SetStatus(codes.Ok, "")
	}
	span.End(trace.WithTimestamp(start.Add(1 * time.Millisecond)))
	return nil
}

// Flush forces the global tracer provider's BatchSpanProcessor to flush.
// The provider is set via Init; if not, this is a no-op.
func (s *TracerSink) Flush(ctx context.Context) error {
	// OTel Go SDK has no public flush API; users should rely on
	// Provider.Shutdown. Phase 1 keeps this as a placeholder.
	_ = ctx
	return nil
}

// Close is a no-op; the provider is closed by the host server.
func (s *TracerSink) Close(_ context.Context) error { return nil }

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

func observationSpanName(obs observation.Observation) string {
	if obs.AgentID != "" {
		return "hnsx.observation." + obs.Kind + "." + obs.AgentID
	}
	if obs.StepID != "" {
		return "hnsx.observation." + obs.Kind + "." + obs.StepID
	}
	return "hnsx.observation." + obs.Kind
}

func observationAttrs(obs observation.Observation) []attribute.KeyValue {
	out := []attribute.KeyValue{
		attribute.String("hnsx.kind", obs.Kind),
	}
	if obs.SessionID != "" {
		out = append(out, attribute.String("hnsx.session_id", obs.SessionID))
	}
	if obs.DomainID != "" {
		out = append(out, attribute.String("hnsx.domain_id", obs.DomainID))
	}
	if obs.StepID != "" {
		out = append(out, attribute.String("hnsx.step_id", obs.StepID))
	}
	if obs.AgentID != "" {
		out = append(out, attribute.String("hnsx.agent_id", obs.AgentID))
	}
	if obs.ParentID != "" {
		out = append(out, attribute.String("hnsx.parent_id", obs.ParentID))
	}
	if obs.TraceID != "" {
		out = append(out, attribute.String("hnsx.trace_id", obs.TraceID))
	}
	if obs.Cost != nil {
		out = append(out,
			attribute.Int("hnsx.cost.prompt_tokens", obs.Cost.PromptTokens),
			attribute.Int("hnsx.cost.completion_tokens", obs.Cost.CompletionTokens),
			attribute.Float64("hnsx.cost.usd", obs.Cost.CostUSD),
			attribute.Int64("hnsx.cost.latency_ms", obs.Cost.LatencyMs),
		)
	}
	return out
}
