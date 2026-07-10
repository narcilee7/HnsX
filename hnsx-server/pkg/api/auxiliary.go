package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/app/queries"
	evalmodel "github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	tracemodel "github.com/hnsx-io/hnsx/server/internal/trace/model"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

// ListTraces handles GET /api/v1/traces — returns the per-domain trace index.
func (s *Server) ListTraces(c *gin.Context) {
	domainFilter := c.Query("domain")
	limit := intQueryGin(c, "limit", 50)
	offset := intQueryGin(c, "offset", 0)

	items := s.Queries.ListSessions(tenantFromGin(c))
	out := make([]map[string]any, 0, len(items))
	for _, sess := range items {
		if domainFilter != "" && sess.DomainID != domainFilter {
			continue
		}
		out = append(out, map[string]any{
			"trace_id":       sess.ID,
			"session_id":     sess.ID,
			"domain_id":      sess.DomainID,
			"domain_version": sess.DomainVersion,
			"status":         sess.State,
			"started_at":     queries.FormatTimeValue(sess.StartedAt),
			"duration_ms":    durationMs(sess),
		})
	}
	// Apply offset/limit naively (full impl needs cursor).
	if offset > len(out) {
		offset = len(out)
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items":  out[offset:end],
		"total":  len(out),
		"limit":  limit,
		"offset": offset,
	})
}

// GetTrace handles GET /api/v1/traces/:traceId.
func (s *Server) GetTrace(c *gin.Context) {
	id := c.Param("traceId")
	sess, ok := s.Queries.GetSession(tenantFromGin(c), id)
	if !ok {
		writeError(c, NewSessionNotFound(id))
		return
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"trace_id":      sess.ID,
		"session_id":    sess.ID,
		"domain_id":     sess.DomainID,
		"orchestration": sess.Orchestration,
		"state":         sess.State,
		"started_at":    sess.StartedAt,
		"duration_ms":   registeredSessionSummary(sess)["duration_ms"],
	})
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
		Orchestration: domain.Spec.Harness.Session.Mode,
		State:         "running",
		TotalCases:    len(set.Cases),
	}
	if err := s.EvalService.CreateRun(run); err != nil {
		writeError(c, NewInternal(err))
		return
	}

	// Phase 1 skeleton: mark the run completed with a neutral score. The actual
	// eval execution loop will be implemented once the worker pipeline can run
	// sessions in batch.
	_ = s.EvalService.FinishRun(run.ID, 0.0, 0, run.TotalCases, 0, 0)

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

// ListRuntimes handles GET /api/v1/runtimes.
func (s *Server) ListRuntimes(c *gin.Context) {
	writeJSON(c, http.StatusOK, map[string]any{
		"items": []map[string]any{
			{
				"runtime_id":        "local-control-plane",
				"version":           "phase1",
				"region":            "local",
				"status":            "active",
				"last_heartbeat_at": time.Now().UTC().Format(time.RFC3339),
			},
		},
		"total": 1,
	})
}

// ListSecrets handles GET /api/v1/secrets — never returns secret *values*.
func (s *Server) ListSecrets(c *gin.Context) {
	writeJSON(c, http.StatusOK, map[string]any{
		"items": []map[string]any{},
		"total": 0,
	})
}

// CreateSecret handles POST /api/v1/secrets.
func (s *Server) CreateSecret(c *gin.Context) {
	writeError(c, &APIError{
		Code:    "ADAPTER_NOT_IMPLEMENTED",
		Message: "secret store not yet implemented (target: Phase 2)",
	})
}

// UpdateSecret handles PUT /api/v1/secrets/:id.
func (s *Server) UpdateSecret(c *gin.Context) {
	writeError(c, &APIError{
		Code:    "ADAPTER_NOT_IMPLEMENTED",
		Message: "secret store not yet implemented (target: Phase 2)",
	})
}

// DeleteSecret handles DELETE /api/v1/secrets/:id.
func (s *Server) DeleteSecret(c *gin.Context) {
	writeError(c, &APIError{
		Code:    "ADAPTER_NOT_IMPLEMENTED",
		Message: "secret store not yet implemented (target: Phase 2)",
	})
}

// ListPolicies handles GET /api/v1/policies.
func (s *Server) ListPolicies(c *gin.Context) {
	writeJSON(c, http.StatusOK, map[string]any{
		"items": []map[string]any{},
		"total": 0,
	})
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
