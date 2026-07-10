package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/app/queries"
	evalmodel "github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	evalrunner "github.com/hnsx-io/hnsx/server/internal/evaluation/runner"
	policymodel "github.com/hnsx-io/hnsx/server/internal/policy/model"
	secmodel "github.com/hnsx-io/hnsx/server/internal/secret/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	tracemodel "github.com/hnsx-io/hnsx/server/internal/trace/model"
	workerpkg "github.com/hnsx-io/hnsx/server/internal/worker"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

// ListTraces handles GET /api/v1/traces — page of TraceSummary records
// computed directly from the observations table. The shape is the
// authoritative wire contract for the console Traces page; trace_id and
// session_id are kept distinct even though today's worker convention
// keeps them equal 1:1.
func (s *Server) ListTraces(c *gin.Context) {
	limit := intQueryGin(c, "limit", 50)
	offset := intQueryGin(c, "offset", 0)
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	from, hasFrom := parseTimeQuery(c.Query("from"))
	to, hasTo := parseTimeQuery(c.Query("to"))

	filter := tracemodel.TraceListFilter{
		TenantID:  string(tenantFromGin(c)),
		DomainID:  c.Query("domain"),
		SessionID: c.Query("session"),
		AgentID:   c.Query("agent"),
		Limit:     limit,
		Offset:    offset,
	}
	if hasFrom {
		filter.From = from
	}
	if hasTo {
		filter.To = to
	}

	if s.TraceService == nil {
		writeJSON(c, http.StatusOK, map[string]any{
			"items":  []map[string]any{},
			"total":  0,
			"limit":  limit,
			"offset": offset,
		})
		return
	}
	res, err := s.TraceService.ListSummaries(filter)
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}
	out := make([]map[string]any, 0, len(res.Summaries))
	for _, sum := range res.Summaries {
		out = append(out, traceSummaryToJSON(sum))
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items":  out,
		"total":  res.Total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetTrace handles GET /api/v1/traces/:traceId — returns the trace envelope
// with the full observation list and a per-trace rollup. 404 with the
// stable TRACE_NOT_FOUND code when the trace_id has no observations.
func (s *Server) GetTrace(c *gin.Context) {
	id := c.Param("traceId")
	if s.TraceService == nil {
		writeError(c, &APIError{
			Code:    "TRACE_NOT_FOUND",
			Message: fmt.Sprintf("trace '%s' not found", id),
		})
		return
	}
	detail, err := s.TraceService.Detail(id)
	if err != nil {
		if errors.Is(err, tracemodel.ErrTraceNotFound) {
			writeError(c, &APIError{
				Code:    "TRACE_NOT_FOUND",
				Message: fmt.Sprintf("trace '%s' not found", id),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	observations := make([]map[string]any, 0, len(detail.Observations))
	for _, rec := range detail.Observations {
		observations = append(observations, observationToMap(rec))
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"trace_id":      detail.TraceID,
		"session_id":    detail.SessionID,
		"domain_id":     detail.DomainID,
		"domain_version": detail.DomainVersion,
		"status":        detail.Status,
		"started_at":    formatTimePtr(detail.StartedAt),
		"completed_at":  formatTimePtr(detail.CompletedAt),
		"duration_ms":   detail.DurationMs,
		"observation_count":     detail.ObservationCount,
		"total_cost_usd":        detail.TotalCostUSD,
		"prompt_tokens":         detail.TotalPromptTokens,
		"completion_tokens":     detail.TotalCompletionTokens,
		"agent_invocations":     detail.AgentInvocations,
		"tool_invocations":      detail.ToolInvocations,
		"observations":          observations,
	})
}

func traceSummaryToJSON(sum tracemodel.TraceSummary) map[string]any {
	out := map[string]any{
		"trace_id":          sum.TraceID,
		"session_id":        sum.SessionID,
		"domain_id":         sum.DomainID,
		"domain_version":    sum.DomainVersion,
		"status":            sum.Status,
		"started_at":        formatTimePtr(sum.StartedAt),
		"completed_at":      formatTimePtr(sum.CompletedAt),
		"duration_ms":       sum.DurationMs,
		"observation_count": sum.ObservationCount,
		"total_cost_usd":    sum.TotalCostUSD,
		"prompt_tokens":     sum.TotalPromptTokens,
		"completion_tokens": sum.TotalCompletionTokens,
		"agent_invocations": sum.AgentInvocations,
		"tool_invocations":  sum.ToolInvocations,
	}
	return out
}

func formatTimePtr(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// ListApprovals handles GET /api/v1/approvals.
func (s *Server) ListApprovals(c *gin.Context) {
	writeJSON(c, http.StatusOK, map[string]any{
		"items":  []map[string]any{},
		"total":  0,
		"limit":  0,
		"offset": 0,
	})
}

// ApproveApproval handles POST /api/v1/approvals/:id/approve.
func (s *Server) ApproveApproval(c *gin.Context) {
	writeError(c, &APIError{
		Code:    "APPROVAL_NOT_FOUND",
		Message: "approval subsystem not enabled in this build",
	})
}

// RejectApproval handles POST /api/v1/approvals/:id/reject.
func (s *Server) RejectApproval(c *gin.Context) {
	writeError(c, &APIError{
		Code:    "APPROVAL_NOT_FOUND",
		Message: "approval subsystem not enabled in this build",
	})
}

// ListEvalSets handles GET /api/v1/evals.
func (s *Server) ListEvalSets(c *gin.Context) {
	limit := intQueryGin(c, "limit", 50)
	offset := intQueryGin(c, "offset", 0)
	if limit <= 0 {
		limit = 50
	}

	if s.EvalService == nil {
		writeJSON(c, http.StatusOK, map[string]any{
			"items":  []map[string]any{},
			"total":  0,
			"limit":  limit,
			"offset": offset,
		})
		return
	}

	sets, total, err := s.EvalService.ListSets(limit, offset)
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}

	out := make([]map[string]any, 0, len(sets))
	for _, set := range sets {
		out = append(out, map[string]any{
			"id":          set.ID,
			"set_id":      set.SetID,
			"domain_id":   set.DomainID,
			"description": set.Description,
			"case_count":  len(set.Cases),
			"created_at":  queries.FormatTimeValue(set.CreatedAt),
		})
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"items":  out,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// CreateEvalSet handles POST /api/v1/evals.
func (s *Server) CreateEvalSet(c *gin.Context) {
	if s.EvalService == nil {
		writeError(c, NewInternal(errors.New("eval service not configured")))
		return
	}

	var body struct {
		SetID       string               `json:"set_id"`
		DomainID    string               `json:"domain_id"`
		Description string               `json:"description,omitempty"`
		Cases       []evalmodel.EvalCase `json:"cases"`
	}
	if err := decodeJSONBody(c, &body); err != nil {
		writeError(c, NewValidation(err))
		return
	}
	if body.SetID == "" || body.DomainID == "" {
		writeError(c, NewValidation(errors.New("set_id and domain_id are required")))
		return
	}
	_, _, ok := s.Queries.GetDomain(tenantFromGin(c), body.DomainID)
	if !ok {
		writeError(c, NewDomainNotFound(body.DomainID))
		return
	}

	set := &evalmodel.EvalSet{
		ID:          body.SetID,
		SetID:       body.SetID,
		DomainID:    body.DomainID,
		Description: body.Description,
		Cases:       body.Cases,
	}
	if err := s.EvalService.CreateSet(set); err != nil {
		writeError(c, NewInternal(err))
		return
	}

	c.Header("Location", "/api/v1/evals/"+set.ID)
	writeJSON(c, http.StatusCreated, map[string]any{
		"id":         set.ID,
		"set_id":     set.SetID,
		"domain_id":  set.DomainID,
		"created_at": queries.FormatTimeValue(set.CreatedAt),
	})
}

// GetEvalSet handles GET /api/v1/evals/:setId.
func (s *Server) GetEvalSet(c *gin.Context) {
	id := c.Param("setId")
	if s.EvalService == nil {
		writeError(c, &APIError{
			Code:    "EVAL_SET_NOT_FOUND",
			Message: "eval service not configured",
		})
		return
	}

	set, err := s.EvalService.GetSet(id)
	if err != nil {
		if errors.Is(err, evalmodel.ErrEvalSetNotFound) {
			writeError(c, &APIError{
				Code:    "EVAL_SET_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	cases := make([]map[string]any, 0, len(set.Cases))
	for _, c := range set.Cases {
		cases = append(cases, map[string]any{
			"id":     c.ID,
			"name":   c.Name,
			"input":  c.Input,
			"expect": c.Expect,
			"scorer": c.Scorer,
		})
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"id":          set.ID,
		"set_id":      set.SetID,
		"domain_id":   set.DomainID,
		"description": set.Description,
		"cases":       cases,
		"created_at":  queries.FormatTimeValue(set.CreatedAt),
		"updated_at":  queries.FormatTimeValue(set.UpdatedAt),
	})
}

// RunEval handles POST /api/v1/evals/:setId/run.
func (s *Server) RunEval(c *gin.Context) {
	id := c.Param("setId")
	if s.EvalService == nil {
		writeError(c, &APIError{
			Code:    "ADAPTER_NOT_IMPLEMENTED",
			Message: "eval runner not yet implemented (target: Phase 2)",
		})
		return
	}

	set, err := s.EvalService.GetSet(id)
	if err != nil {
		if errors.Is(err, evalmodel.ErrEvalSetNotFound) {
			writeError(c, &APIError{
				Code:    "EVAL_SET_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	_, domain, ok := s.Queries.GetDomain(tenantFromGin(c), set.DomainID)
	if !ok {
		writeError(c, NewDomainNotFound(set.DomainID))
		return
	}

	run := &evalmodel.EvalRun{
		ID:            runtime.NewSessionID(set.ID),
		EvalSetID:     set.ID,
		DomainID:      set.DomainID,
		DomainVersion: domain.Version,
		Orchestration: string(domain.Spec.Harness.Session.Mode),
		State:         "running",
		TotalCases:    len(set.Cases),
	}
	if err := s.EvalService.CreateRun(run); err != nil {
		writeError(c, NewInternal(err))
		return
	}

	// The eval runner drives one synchronous session per case via the local
	// executor. When the server runs in pure worker-pool mode (no executor),
	// batch eval is not yet available.
	if s.Executor == nil {
		writeError(c, &APIError{
			Code:    "ADAPTER_NOT_IMPLEMENTED",
			Message: "eval runner requires the local executor in this build",
		})
		return
	}

	specForRun := domain.Spec
	budget := 0.0
	if specForRun != nil {
		budget = specForRun.Harness.Policy.Budget.MaxCostUSD
	}
	traceSvc := s.TraceService
	er := evalrunner.New(s.Executor, s.EvalService, evalrunner.WithCostFunc(func(sessionID string) float64 {
		if traceSvc == nil {
			return 0
		}
		agg, err := traceSvc.Aggregate([]string{sessionID})
		if err != nil {
			return 0
		}
		return agg.TotalCostUSD
	}))
	tenantID := tenantFromGin(c)
	go func() {
		ctx := tenant.NewContext(context.Background(), tenantID)
		_ = er.Run(ctx, run, set, specForRun, budget)
	}()

	c.Header("Location", fmt.Sprintf("/api/v1/evals/%s/runs/%s", set.ID, run.ID))
	writeJSON(c, http.StatusAccepted, map[string]any{
		"run_id": run.ID,
		"state":  "running",
	})
}

// GetEvalRun handles GET /api/v1/evals/:setId/runs/:runId.
func (s *Server) GetEvalRun(c *gin.Context) {
	runID := c.Param("runId")
	if s.EvalService == nil {
		writeError(c, &APIError{
			Code:    "ADAPTER_NOT_IMPLEMENTED",
			Message: "eval runner not yet implemented (target: Phase 2)",
		})
		return
	}

	run, err := s.EvalService.GetRun(runID)
	if err != nil {
		if errors.Is(err, evalmodel.ErrEvalRunNotFound) {
			writeError(c, &APIError{
				Code:    "EVAL_RUN_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	cases := make([]map[string]any, 0, len(run.Results))
	for _, res := range run.Results {
		cases = append(cases, map[string]any{
			"case_id":     res.CaseID,
			"session_id":  res.SessionID,
			"score":       res.Score,
			"passed":      res.Passed,
			"actual":      res.Actual,
			"details":     res.Details,
			"duration_ms": res.DurationMs,
			"cost_usd":    res.CostUSD,
		})
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"id":             run.ID,
		"eval_set_id":    run.EvalSetID,
		"domain_id":      run.DomainID,
		"domain_version": run.DomainVersion,
		"orchestration":  run.Orchestration,
		"state":          run.State,
		"score":          run.Score,
		"total_cases":    run.TotalCases,
		"passed_cases":   run.PassedCases,
		"total_cost_usd": run.TotalCostUSD,
		"duration_ms":    run.DurationMs,
		"cases":          cases,
		"created_at":     queries.FormatTimeValue(run.CreatedAt),
		"completed_at":   queries.FormatTime(run.CompletedAt),
	})
}

// ListAudit handles GET /api/v1/audit.
func (s *Server) ListAudit(c *gin.Context) {
	limit := intQueryGin(c, "limit", 50)
	offset := intQueryGin(c, "offset", 0)
	if limit <= 0 {
		limit = 50
	}

	if s.AuditService == nil {
		writeJSON(c, http.StatusOK, map[string]any{
			"items":  []map[string]any{},
			"total":  0,
			"limit":  limit,
			"offset": offset,
		})
		return
	}

	entries, total, err := s.AuditService.List(limit, offset)
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}

	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"id":            e.ID,
			"session_id":    e.SessionID,
			"domain_id":     e.DomainID,
			"action":        e.Action,
			"actor":         e.Actor,
			"actor_type":    e.ActorType,
			"resource":      e.Resource,
			"resource_type": e.ResourceType,
			"decision":      e.Decision,
			"reason":        e.Reason,
			"details":       e.Details,
			"timestamp":     e.Timestamp.Format(time.RFC3339),
		})
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"items":  out,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetMetrics handles GET /api/v1/metrics.
func (s *Server) GetMetrics(c *gin.Context) {
	domainFilter := c.Query("domain")
	sessions := s.Queries.ListSessions(tenantFromGin(c))

	total := 0
	completed := 0
	failed := 0
	var totalDurationMs uint64
	var sessionIDs []string
	for _, sess := range sessions {
		if domainFilter != "" && sess.DomainID != domainFilter {
			continue
		}
		total++
		switch sess.State {
		case "completed":
			completed++
		case "failed":
			failed++
		}
		totalDurationMs += durationMs(sess)
		sessionIDs = append(sessionIDs, sess.ID)
	}

	var agg tracemodel.Aggregate
	if s.TraceService != nil {
		var err error
		agg, err = s.TraceService.Aggregate(sessionIDs)
		if err != nil {
			writeError(c, NewInternal(err))
			return
		}
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"domain_id":          domainFilter,
		"total_sessions":     total,
		"completed_sessions": completed,
		"failed_sessions":    failed,
		"total_cost_usd":     agg.TotalCostUSD,
		"avg_duration_ms":    avgDurationMs(totalDurationMs, total),
		"agent_invocations":  agg.AgentInvocations,
		"tool_invocations":   agg.ToolInvocations,
		"prompt_tokens":      agg.TotalPromptTokens,
		"completion_tokens":  agg.TotalCompletionTokens,
	})
}

// ListRuntimes handles GET /api/v1/runtimes — returns every live runtime
// worker that has registered with the control plane, with a freshness
// projection. WorkerService is only wired when the gRPC control plane is
// enabled (cfg.GRPCAddr != ""); in pure HTTP mode we return an empty list
// instead of a hand-rolled placeholder so the UI can render the "no
// workers yet" empty state honestly.
func (s *Server) ListRuntimes(c *gin.Context) {
	if s.WorkerService == nil {
		writeJSON(c, http.StatusOK, map[string]any{
			"items": []map[string]any{},
			"total": 0,
		})
		return
	}
	snaps := s.WorkerService.List()
	items := make([]map[string]any, 0, len(snaps))
	for _, snap := range snaps {
		items = append(items, runtimeSnapshotToJSON(snap))
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

func runtimeSnapshotToJSON(snap workerpkg.Snapshot) map[string]any {
	out := map[string]any{
		"runtime_id":        snap.WorkerID,
		"status":            runtimeStatus(snap),
		"last_heartbeat_at": snap.LastSeen.UTC().Format(time.RFC3339Nano),
		"age_seconds":       snap.AgeSeconds,
		"healthy":           snap.Healthy,
	}
	if snap.Info == nil {
		return out
	}
	if snap.Info.Version != "" {
		out["version"] = snap.Info.Version
	}
	if snap.Info.Region != "" {
		out["region"] = snap.Info.Region
	}
	if snap.Info.Hostname != "" {
		out["hostname"] = snap.Info.Hostname
	}
	if snap.Info.Pid != "" {
		out["pid"] = snap.Info.Pid
	}
	if snap.Info.Capacity != nil {
		if snap.Info.Capacity.MaxConcurrentSessions > 0 {
			out["capacity"] = snap.Info.Capacity.MaxConcurrentSessions
		}
		if len(snap.Info.Capacity.Providers) > 0 {
			out["capabilities"] = append([]string(nil), snap.Info.Capacity.Providers...)
		}
		if len(snap.Info.Capacity.Models) > 0 {
			out["models"] = append([]string(nil), snap.Info.Capacity.Models...)
		}
		if len(snap.Info.Capacity.SandboxRuntimes) > 0 {
			out["sandbox_runtimes"] = append([]string(nil), snap.Info.Capacity.SandboxRuntimes...)
		}
	}
	if len(snap.Info.Labels) > 0 {
		labels := make(map[string]string, len(snap.Info.Labels))
		for k, v := range snap.Info.Labels {
			labels[k] = v
		}
		out["labels"] = labels
	}
	return out
}

// runtimeStatus maps the registry's liveness signal into the wire-level
// status vocabulary that the UI expects:
//   - healthy       : last heartbeat within 30s
//   - degraded      : older but not yet evicted
//   - offline       : beyond the soft-eviction threshold (60s)
// Note: the registry's EvictStale policy decides when a worker is
// actually removed; this function only labels, it does not mutate state.
func runtimeStatus(snap workerpkg.Snapshot) string {
	if !snap.LastSeen.IsZero() && snap.AgeSeconds > 60 {
		return "offline"
	}
	if snap.Healthy {
		return "healthy"
	}
	return "degraded"
}

// ListSecrets handles GET /api/v1/secrets. The wire payload contains no
// Value/PlainValue field by design; operators verify a typed value via
// the last-4 fingerprint, never the plaintext itself.
func (s *Server) ListSecrets(c *gin.Context) {
	if s.SecretService == nil {
		writeError(c, &APIError{
			Code:    "SECRETS_UNAVAILABLE",
			Message: "secret store is not configured on this server",
		})
		return
	}
	items, err := s.SecretService.List()
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{
			"name":        it.Name,
			"description": it.Description,
			"kind":        it.Kind,
			"fingerprint": it.Fingerprint,
			"created_at":  it.CreatedAt,
			"updated_at":  it.UpdatedAt,
		})
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items": out,
		"total": len(out),
	})
}

// secretWriteBody is the request shape for Create / Update. Both calls
// accept a fresh plaintext value; the response never echoes it back.
type secretWriteBody struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
}

// CreateSecret handles POST /api/v1/secrets. The plaintext Value is
// required; the cipher encrypts it before it touches the repository, and
// the response is a fingerprint-only ack (no Value field).
func (s *Server) CreateSecret(c *gin.Context) {
	if s.SecretService == nil {
		writeError(c, &APIError{
			Code:    "SECRETS_UNAVAILABLE",
			Message: "secret store is not configured on this server",
		})
		return
	}
	var body secretWriteBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	if body.Name == "" || body.Value == "" {
		writeError(c, NewInvalidRequest("name and value are required"))
		return
	}
	sec := &secmodel.Secret{
		Name:        body.Name,
		Description: body.Description,
		Kind:        body.Kind,
		PlainValue:  body.Value,
	}
	if err := s.SecretService.Save(sec); err != nil {
		writeError(c, NewInternal(err))
		return
	}
	writeJSON(c, http.StatusCreated, map[string]any{
		"name":        sec.Name,
		"description": sec.Description,
		"kind":        sec.Kind,
		"fingerprint": sec.Fingerprint,
	})
}

// UpdateSecret handles PUT /api/v1/secrets/:id — replaces the value +
// metadata. Body shape is identical to Create; name in the URL is the
// authority.
func (s *Server) UpdateSecret(c *gin.Context) {
	if s.SecretService == nil {
		writeError(c, &APIError{
			Code:    "SECRETS_UNAVAILABLE",
			Message: "secret store is not configured on this server",
		})
		return
	}
	id := c.Param("id")
	var body secretWriteBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	if body.Value == "" {
		writeError(c, NewInvalidRequest("value is required for update"))
		return
	}
	sec := &secmodel.Secret{
		Name:        id,
		Description: body.Description,
		Kind:        body.Kind,
		PlainValue:  body.Value,
	}
	if err := s.SecretService.Save(sec); err != nil {
		writeError(c, NewInternal(err))
		return
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"name":        sec.Name,
		"description": sec.Description,
		"kind":        sec.Kind,
		"fingerprint": sec.Fingerprint,
	})
}

// DeleteSecret handles DELETE /api/v1/secrets/:id.
func (s *Server) DeleteSecret(c *gin.Context) {
	if s.SecretService == nil {
		writeError(c, &APIError{
			Code:    "SECRETS_UNAVAILABLE",
			Message: "secret store is not configured on this server",
		})
		return
	}
	if err := s.SecretService.Delete(c.Param("id")); err != nil {
		writeError(c, NewInternal(err))
		return
	}
	c.AbortWithStatus(http.StatusNoContent)
}

// ListPolicies handles GET /api/v1/policies.
func (s *Server) ListPolicies(c *gin.Context) {
	if s.PolicyService == nil {
		writeError(c, &APIError{
			Code:    "POLICY_UNAVAILABLE",
			Message: "policy service is not configured on this server",
		})
		return
	}
	items, err := s.PolicyService.List()
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, policyItemToJSON(it))
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items": out,
		"total": len(out),
	})
}

// CreatePolicy handles POST /api/v1/policies. Body shape:
//
//	{ "id": "no-shell",
//	  "name": "Deny Shell",
//	  "description": "...",
//	  "budget": { "max_cost_usd": 10, ... },
//	  "permissions": { ... },
//	  "guardrails": [ ... ] }
//
// id is required; the rest default to safe zero values. BoundDomain is
// intentionally NOT accepted here — bindings go through
// /domains/:id/policies so the dedicated route owns that relationship.
func (s *Server) CreatePolicy(c *gin.Context) {
	if s.PolicyService == nil {
		writeError(c, &APIError{
			Code:    "POLICY_UNAVAILABLE",
			Message: "policy service is not configured on this server",
		})
		return
	}
	var body policyWriteBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	if body.ID == "" {
		writeError(c, NewInvalidRequest("id is required"))
		return
	}
	if body.Name == "" {
		body.Name = body.ID
	}
	policy := &policymodel.Policy{
		ID:          body.ID,
		Name:        body.Name,
		Description: body.Description,
		Budget:      body.Budget,
		Permissions: body.Permissions,
		Guardrails:  body.Guardrails,
	}
	if err := s.PolicyService.CreateOrUpdate(policy); err != nil {
		writeError(c, NewInternal(err))
		return
	}
	writeJSON(c, http.StatusCreated, policyToJSON(policy))
}

// UpdatePolicy handles PUT /api/v1/policies/:id. Same body as Create.
func (s *Server) UpdatePolicy(c *gin.Context) {
	if s.PolicyService == nil {
		writeError(c, &APIError{
			Code:    "POLICY_UNAVAILABLE",
			Message: "policy service is not configured on this server",
		})
		return
	}
	id := c.Param("id")
	var body policyWriteBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	if body.ID != "" && body.ID != id {
		writeError(c, NewInvalidRequest("id in body must match id in url"))
		return
	}
	policy := &policymodel.Policy{
		ID:          id,
		Name:        body.Name,
		Description: body.Description,
		Budget:      body.Budget,
		Permissions: body.Permissions,
		Guardrails:  body.Guardrails,
	}
	if policy.Name == "" {
		policy.Name = id
	}
	if err := s.PolicyService.CreateOrUpdate(policy); err != nil {
		writeError(c, NewInternal(err))
		return
	}
	writeJSON(c, http.StatusOK, policyToJSON(policy))
}

// DeletePolicy handles DELETE /api/v1/policies/:id.
func (s *Server) DeletePolicy(c *gin.Context) {
	if s.PolicyService == nil {
		writeError(c, &APIError{
			Code:    "POLICY_UNAVAILABLE",
			Message: "policy service is not configured on this server",
		})
		return
	}
	if err := s.PolicyService.Delete(c.Param("id")); err != nil {
		if errors.Is(err, policymodel.ErrPolicyNotFound) {
			writeError(c, &APIError{
				Code:    "POLICY_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}
	c.AbortWithStatus(http.StatusNoContent)
}

// BindPolicy handles POST /api/v1/domains/:id/policies — associates
// the named policy with the domain. Body: { "policy_id": "no-shell" }.
// If the domain already had a different policy bound, that binding is
// replaced (so the 1:1 invariant holds).
func (s *Server) BindPolicy(c *gin.Context) {
	if s.PolicyService == nil {
		writeError(c, &APIError{
			Code:    "POLICY_UNAVAILABLE",
			Message: "policy service is not configured on this server",
		})
		return
	}
	domainID := c.Param("id")
	var body struct {
		PolicyID string `json:"policy_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	if body.PolicyID == "" {
		writeError(c, NewInvalidRequest("policy_id is required"))
		return
	}
	if err := s.PolicyService.BindDomain(body.PolicyID, domainID); err != nil {
		if errors.Is(err, policymodel.ErrPolicyNotFound) {
			writeError(c, &APIError{
				Code:    "POLICY_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"domain_id": domainID,
		"policy_id": body.PolicyID,
	})
}

type policyWriteBody struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Budget      policymodel.Budget     `json:"budget"`
	Permissions policymodel.Permissions `json:"permissions"`
	Guardrails  []policymodel.Guardrail `json:"guardrails"`
}

func policyItemToJSON(it policymodel.ListItem) map[string]any {
	return map[string]any{
		"id":           it.ID,
		"name":         it.Name,
		"description":  it.Description,
		"bound_domain": it.BoundDomain,
		"budget":       it.Budget,
		"permissions":  it.Permissions,
		"guardrails":   it.Guardrails,
		"created_at":   it.CreatedAt,
		"updated_at":   it.UpdatedAt,
	}
}

func policyToJSON(p *policymodel.Policy) map[string]any {
	return map[string]any{
		"id":           p.ID,
		"name":         p.Name,
		"description":  p.Description,
		"bound_domain": p.BoundDomain,
		"budget":       p.Budget,
		"permissions":  p.Permissions,
		"guardrails":   p.Guardrails,
		"created_at":   p.CreatedAt,
		"updated_at":   p.UpdatedAt,
	}
}

// ----------------------------------------------------------------------------
// tiny helpers
// ----------------------------------------------------------------------------

func intQueryGin(c *gin.Context, key string, def int) int {
	v := c.Query(key)
	if v == "" {
		return def
	}
	n := 0
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func durationMs(sess queries.SessionListItem) uint64 {
	if sess.CompletedAt == nil {
		return 0
	}
	delta := sess.CompletedAt.Sub(sess.StartedAt).Milliseconds()
	if delta < 0 {
		return 0
	}
	return uint64(delta)
}

func avgDurationMs(total uint64, n int) float64 {
	if n == 0 {
		return 0
	}
	return float64(total) / float64(n)
}

// parseTimeQuery parses an RFC3339 timestamp or a bare YYYY-MM-DD date. The
// bool reports whether a usable value was parsed.
func parseTimeQuery(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// observationToMap renders a persisted observation record as the JSON shape used
// by the trace endpoints.
func observationToMap(rec tracemodel.ObservationRecord) map[string]any {
	return map[string]any{
		"kind":              rec.Kind,
		"trace_id":          rec.TraceID,
		"session_id":        rec.SessionID,
		"domain_id":         rec.DomainID,
		"domain_version":    rec.DomainVersion,
		"step_id":           rec.StepID,
		"agent_id":          rec.AgentID,
		"payload":           rec.Payload,
		"cost_usd":          rec.CostUSD,
		"prompt_tokens":     rec.PromptTokens,
		"completion_tokens": rec.CompletionTokens,
		"latency_ms":        rec.LatencyMs,
		"timestamp":         queries.FormatTimeValue(rec.CreatedAt),
	}
}
