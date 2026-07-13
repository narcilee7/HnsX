package handler

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/hnsx-io/hnsx/server/internal/tenant"
	tracemodel "github.com/hnsx-io/hnsx/server/internal/trace/model"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
	"github.com/hnsx-io/hnsx/server/pkg/handler/viewmodel"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

type ListTracesInput struct {
	TenantID  tenant.ID
	DomainID  string
	SessionID string
	AgentID   string
	From      time.Time
	To        time.Time
	Limit     int
	Offset    int
}

type GetTraceInput struct {
	TenantID tenant.ID
	TraceID  string
}

type RecordTraceInput struct {
	TenantID     tenant.ID
	TraceID      string
	SessionID    string
	DomainID     string
	DomainVersion string
	Observations []domain.Observation
}

type QueryTracesInput struct {
	TenantID  tenant.ID
	TraceID   string
	DomainID  string
	SessionID string
	Limit     int
}

type RecordInvocationInput struct {
	TenantID         tenant.ID
	SessionID        string
	DomainID         string
	TotalCostUSD     float64
	PromptTokens     int64
	CompletionTokens int64
	DurationMs       int64
}

type QueryInvocationMetricsInput struct {
	TenantID tenant.ID
	DomainID string
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

type ListTracesOutput struct {
	Traces viewmodel.TraceList
}

type GetTraceOutput struct {
	Trace *viewmodel.TraceDetail
}

type QueryTracesOutput struct {
	Traces []*viewmodel.TraceDetail
}

type QueryInvocationMetricsOutput struct {
	DomainID              string
	InvocationCount       int64
	TotalCostUSD          float64
	TotalPromptTokens     int64
	TotalCompletionTokens int64
	AvgLatencyMs          float64
}

// ---------------------------------------------------------------------------
// Handler methods
// ---------------------------------------------------------------------------

// ListTraces returns trace summaries matching the supplied filters.
func (h *Handler) ListTraces(ctx context.Context, in ListTracesInput) (*ListTracesOutput, error) {
	defer h.hook(ctx, "trace.list", zap.String("tenant_id", string(in.TenantID)))()

	if h.App == nil || h.App.TraceService == nil {
		return &ListTracesOutput{Traces: viewmodel.TraceList{}}, nil
	}
	filter := tracemodel.TraceListFilter{
		TenantID:  string(in.TenantID),
		DomainID:  in.DomainID,
		SessionID: in.SessionID,
		AgentID:   in.AgentID,
		Limit:     in.Limit,
		Offset:    in.Offset,
	}
	if !in.From.IsZero() {
		filter.From = in.From
	}
	if !in.To.IsZero() {
		filter.To = in.To
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	result, err := h.App.TraceService.ListSummaries(filter)
	if err != nil {
		return nil, err
	}
	out := make([]viewmodel.TraceListItem, 0, len(result.Summaries))
	for _, s := range result.Summaries {
		out = append(out, viewmodel.TraceListItem{
			TraceID:               s.TraceID,
			SessionID:             s.SessionID,
			DomainID:              s.DomainID,
			DomainVersion:         s.DomainVersion,
			Status:                s.Status,
			StartedAt:             s.StartedAt,
			CompletedAt:           s.CompletedAt,
			DurationMs:            s.DurationMs,
			ObservationCount:      s.ObservationCount,
			TotalCostUSD:          s.TotalCostUSD,
			TotalPromptTokens:     s.TotalPromptTokens,
			TotalCompletionTokens: s.TotalCompletionTokens,
			AgentInvocations:      s.AgentInvocations,
			ToolInvocations:       s.ToolInvocations,
			CreatedAt:             s.StartedAt,
		})
	}
	return &ListTracesOutput{Traces: viewmodel.TraceList{
		Items:  out,
		Total:  result.Total,
		Limit:  filter.Limit,
		Offset: in.Offset,
	}}, nil
}

// GetTrace returns the full trace detail including observations.
func (h *Handler) GetTrace(ctx context.Context, in GetTraceInput) (*GetTraceOutput, error) {
	defer h.hook(ctx, "trace.get",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("trace_id", in.TraceID),
	)()

	if h.App == nil || h.App.TraceService == nil {
		return nil, fmt.Errorf("trace service unavailable")
	}
	detail, err := h.App.TraceService.Detail(in.TraceID)
	if err != nil {
		return nil, err
	}
	obs := make([]viewmodel.ObservationItem, 0, len(detail.Observations))
	for _, o := range detail.Observations {
		obs = append(obs, viewmodel.ObservationItem{
			TraceID:          o.TraceID,
			SessionID:        o.SessionID,
			DomainID:         o.DomainID,
			DomainVersion:    o.DomainVersion,
			StepID:           o.StepID,
			AgentID:          o.AgentID,
			Kind:             o.Kind,
			Payload:          o.Payload,
			Metadata:         o.Metadata,
			CostUSD:          o.CostUSD,
			PromptTokens:     o.PromptTokens,
			CompletionTokens: o.CompletionTokens,
			LatencyMs:        o.LatencyMs,
			CreatedAt:        o.CreatedAt,
		})
	}
	return &GetTraceOutput{Trace: &viewmodel.TraceDetail{
		TraceID:               detail.TraceID,
		SessionID:             detail.SessionID,
		DomainID:              detail.DomainID,
		DomainVersion:         detail.DomainVersion,
		Status:                detail.Status,
		StartedAt:             detail.StartedAt,
		CompletedAt:           detail.CompletedAt,
		DurationMs:            detail.DurationMs,
		ObservationCount:      detail.ObservationCount,
		TotalCostUSD:          detail.TotalCostUSD,
		TotalPromptTokens:     detail.TotalPromptTokens,
		TotalCompletionTokens: detail.TotalCompletionTokens,
		AgentInvocations:      detail.AgentInvocations,
		ToolInvocations:       detail.ToolInvocations,
		Observations:          obs,
	}}, nil
}

// RecordTrace persists a batch of runtime observations.
func (h *Handler) RecordTrace(ctx context.Context, in RecordTraceInput) error {
	defer h.hook(ctx, "trace.record",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("trace_id", in.TraceID),
		zap.String("session_id", in.SessionID),
	)()

	if h.App == nil || h.App.TraceService == nil {
		return fmt.Errorf("trace service unavailable")
	}
	for i := range in.Observations {
		obs := &in.Observations[i]
		if obs.TraceID == "" {
			obs.TraceID = in.TraceID
		}
		if obs.SessionID == "" {
			obs.SessionID = in.SessionID
		}
		if obs.DomainID == "" {
			obs.DomainID = in.DomainID
		}
		if err := h.App.TraceService.Record(ctx, *obs); err != nil {
			return err
		}
	}
	return nil
}

// QueryTraces returns a single trace by ID or a filtered list of trace details.
func (h *Handler) QueryTraces(ctx context.Context, in QueryTracesInput) (*QueryTracesOutput, error) {
	defer h.hook(ctx, "trace.query",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("trace_id", in.TraceID),
		zap.String("session_id", in.SessionID),
	)()

	if h.App == nil || h.App.TraceService == nil {
		return &QueryTracesOutput{}, nil
	}

	if in.TraceID != "" {
		detail, err := h.App.TraceService.Detail(in.TraceID)
		if err != nil {
			return nil, err
		}
		out, err := h.GetTrace(ctx, GetTraceInput{TenantID: in.TenantID, TraceID: detail.TraceID})
		if err != nil {
			return nil, err
		}
		return &QueryTracesOutput{Traces: []*viewmodel.TraceDetail{out.Trace}}, nil
	}

	filter := tracemodel.TraceListFilter{
		TenantID:  string(in.TenantID),
		DomainID:  in.DomainID,
		SessionID: in.SessionID,
		Limit:     in.Limit,
	}
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	summaries, err := h.App.TraceService.ListSummaries(filter)
	if err != nil {
		return nil, err
	}
	traces := make([]*viewmodel.TraceDetail, 0, len(summaries.Summaries))
	for _, s := range summaries.Summaries {
		traces = append(traces, &viewmodel.TraceDetail{
			TraceID:               s.TraceID,
			SessionID:             s.SessionID,
			DomainID:              s.DomainID,
			DomainVersion:         s.DomainVersion,
			Status:                s.Status,
			StartedAt:             s.StartedAt,
			CompletedAt:           s.CompletedAt,
			DurationMs:            s.DurationMs,
			ObservationCount:      s.ObservationCount,
			TotalCostUSD:          s.TotalCostUSD,
			TotalPromptTokens:     s.TotalPromptTokens,
			TotalCompletionTokens: s.TotalCompletionTokens,
			AgentInvocations:      s.AgentInvocations,
			ToolInvocations:       s.ToolInvocations,
		})
	}
	return &QueryTracesOutput{Traces: traces}, nil
}

// RecordInvocation persists a single invocation-level observation.
func (h *Handler) RecordInvocation(ctx context.Context, in RecordInvocationInput) error {
	defer h.hook(ctx, "invocation.record",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("session_id", in.SessionID),
	)()

	if h.App == nil || h.App.TraceService == nil {
		return fmt.Errorf("trace service unavailable")
	}
	obs := domain.Observation{
		Kind:      "invocation",
		SessionID: in.SessionID,
		DomainID:  in.DomainID,
		TraceID:   in.SessionID,
		Timestamp: time.Now().UTC(),
		Cost: &domain.Cost{
			CostUSD:          in.TotalCostUSD,
			PromptTokens:     int(in.PromptTokens),
			CompletionTokens: int(in.CompletionTokens),
			LatencyMs:        in.DurationMs,
		},
	}
	return h.App.TraceService.Record(ctx, obs)
}

// QueryInvocationMetrics returns rolled-up invocation metrics for a domain.
func (h *Handler) QueryInvocationMetrics(ctx context.Context, in QueryInvocationMetricsInput) (*QueryInvocationMetricsOutput, error) {
	defer h.hook(ctx, "invocation.metrics",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("domain_id", in.DomainID),
	)()

	if h.App == nil || h.App.TraceService == nil {
		return &QueryInvocationMetricsOutput{DomainID: in.DomainID}, nil
	}

	var sessionIDs []string
	if in.DomainID != "" && h.App.SessionService != nil {
		sessions, err := h.App.SessionService.ListByDomain(in.TenantID, in.DomainID)
		if err == nil {
			for _, sess := range sessions {
				sessionIDs = append(sessionIDs, sess.ID)
			}
		}
	}

	agg, err := h.App.TraceService.Aggregate(sessionIDs)
	if err != nil {
		return nil, err
	}
	count := agg.AgentInvocations + agg.ToolInvocations
	var avgLatency float64
	if count > 0 {
		avgLatency = 0 // TODO: trace aggregate does not yet expose avg latency
	}
	return &QueryInvocationMetricsOutput{
		DomainID:              in.DomainID,
		InvocationCount:       int64(count),
		TotalCostUSD:          agg.TotalCostUSD,
		TotalPromptTokens:     int64(agg.TotalPromptTokens),
		TotalCompletionTokens: int64(agg.TotalCompletionTokens),
		AvgLatencyMs:          avgLatency,
	}, nil
}

// IsTraceNotFound reports whether err is a trace-not-found error.
func IsTraceNotFound(err error) bool {
	return err != nil && err.Error() == tracemodel.ErrTraceNotFound.Error()
}
