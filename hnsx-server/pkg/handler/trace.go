package handler

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/hnsx-io/hnsx/server/internal/tenant"
	tracemodel "github.com/hnsx-io/hnsx/server/internal/trace/model"
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

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

type ListTracesOutput struct {
	Traces viewmodel.TraceList
}

type GetTraceOutput struct {
	Trace *viewmodel.TraceDetail
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

// IsTraceNotFound reports whether err is a trace-not-found error.
func IsTraceNotFound(err error) bool {
	return err != nil && err.Error() == tracemodel.ErrTraceNotFound.Error()
}
