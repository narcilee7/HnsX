package handler

import (
	"context"
	"errors"

	"go.uber.org/zap"

	sessionmodel "github.com/hnsx-io/hnsx/server/internal/session/model"
	tracemodel "github.com/hnsx-io/hnsx/server/internal/trace/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/handler/viewmodel"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

// GetMetricsInput selects the tenant and optional domain filter for metrics.
type GetMetricsInput struct {
	TenantID tenant.ID
	DomainID string
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

// GetMetricsOutput carries the rolled-up metrics view.
type GetMetricsOutput struct {
	Metrics *viewmodel.Metrics
}

// ---------------------------------------------------------------------------
// Handler methods
// ---------------------------------------------------------------------------

// GetMetrics returns aggregate session metrics, optionally filtered by domain.
// It reproduces the aggregation logic previously living in pkg/api/auxiliary.go.
func (h *Handler) GetMetrics(ctx context.Context, in GetMetricsInput) (*GetMetricsOutput, error) {
	defer h.hook(ctx, "metric.get",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("domain_id", in.DomainID),
	)()

	if h.App == nil || h.App.SessionService == nil {
		return nil, ErrSessionNotFound
	}

	sessions, err := h.App.SessionService.List(in.TenantID)
	if err != nil {
		return nil, err
	}

	total := 0
	completed := 0
	failed := 0
	var totalDurationMs uint64
	sessionIDs := make([]string, 0, len(sessions))
	for _, s := range sessions {
		if in.DomainID != "" && s.DomainID != in.DomainID {
			continue
		}
		total++
		switch s.State {
		case sessionmodel.StateCompleted:
			completed++
		case sessionmodel.StateFailed:
			failed++
		}
		totalDurationMs += metricDurationMs(s)
		sessionIDs = append(sessionIDs, s.ID)
	}

	var agg tracemodel.Aggregate
	if h.App.TraceService != nil {
		agg, err = h.App.TraceService.Aggregate(sessionIDs)
		if err != nil {
			return nil, err
		}
	}

	return &GetMetricsOutput{Metrics: &viewmodel.Metrics{
		DomainID:          in.DomainID,
		TotalSessions:     total,
		CompletedSessions: completed,
		FailedSessions:    failed,
		TotalCostUSD:      agg.TotalCostUSD,
		AvgDurationMs:     metricAvgDurationMs(totalDurationMs, total),
		AgentInvocations:  agg.AgentInvocations,
		ToolInvocations:   agg.ToolInvocations,
		PromptTokens:      agg.TotalPromptTokens,
		CompletionTokens:  agg.TotalCompletionTokens,
	}}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// metricDurationMs returns the session duration in milliseconds, or 0 when
// the session has not completed.
func metricDurationMs(s *sessionmodel.Session) uint64 {
	if s == nil || s.CompletedAt == nil {
		return 0
	}
	delta := s.CompletedAt.Sub(s.StartedAt).Milliseconds()
	if delta < 0 {
		return 0
	}
	return uint64(delta)
}

// metricAvgDurationMs divides total duration by the session count.
func metricAvgDurationMs(total uint64, n int) float64 {
	if n == 0 {
		return 0
	}
	return float64(total) / float64(n)
}

// Metric errors re-exported from the underlying model packages so HTTP/gRPC
// layers can compare directly.
var (
	ErrSessionNotFound = sessionmodel.ErrSessionNotFound
	ErrTraceNotFound   = tracemodel.ErrTraceNotFound
)

// IsMetricNotFound reports whether err is a not-found error from a metric
// dependency (session or trace).
func IsMetricNotFound(err error) bool {
	return errors.Is(err, ErrSessionNotFound) || errors.Is(err, ErrTraceNotFound)
}

// mapMetricError maps a metric service error to a stable handler error.
func mapMetricError(err error) error {
	if err == nil {
		return nil
	}
	if IsMetricNotFound(err) {
		return ErrSessionNotFound
	}
	return err
}
