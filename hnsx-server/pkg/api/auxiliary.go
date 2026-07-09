package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/hnsx-io/hnsx/server/internal/app/queries"
	evalmodel "github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

// ListTraces handles GET /api/v1/traces — returns the per-domain trace index.
func (s *Server) ListTraces(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	domainFilter := q.Get("domain")
	limit := intQuery(r, "limit", 50)
	offset := intQuery(r, "offset", 0)

	items := queries.ListSessions(s.AppState)
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
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  out[offset:end],
		"total":  len(out),
		"limit":  limit,
		"offset": offset,
	})
}

// GetTrace handles GET /api/v1/traces/{traceId}.
func (s *Server) GetTrace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "traceId")
	sess, ok := queries.GetSession(s.AppState, id)
	if !ok {
		writeError(w, r, NewSessionNotFound(id))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"trace_id":      sess.ID,
		"session_id":    sess.ID,
		"domain_id":     sess.DomainID,
		"orchestration": sess.Orchestration,
		"state":         sess.State,
		"started_at":    queries.FormatTimeValue(sess.StartedAt),
		"duration_ms": durationMs(queries.SessionListItem{
			ID:          sess.ID,
			StartedAt:   sess.StartedAt,
			CompletedAt: sess.CompletedAt,
		}),
	})
}

// ListApprovals handles GET /api/v1/approvals.
func (s *Server) ListApprovals(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"items":  []map[string]any{},
		"total":  0,
		"limit":  0,
		"offset": 0,
	})
}

// ApproveApproval handles POST /api/v1/approvals/{id}/approve.
func (s *Server) ApproveApproval(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, &APIError{
		Code:    "APPROVAL_NOT_FOUND",
		Message: "approval subsystem not enabled in this build",
	})
}

// RejectApproval handles POST /api/v1/approvals/{id}/reject.
func (s *Server) RejectApproval(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, &APIError{
		Code:    "APPROVAL_NOT_FOUND",
		Message: "approval subsystem not enabled in this build",
	})
}

// ListEvalSets handles GET /api/v1/evals.
func (s *Server) ListEvalSets(w http.ResponseWriter, r *http.Request) {
	limit := intQuery(r, "limit", 50)
	offset := intQuery(r, "offset", 0)
	if limit <= 0 {
		limit = 50
	}

	if s.EvalService == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"items":  []map[string]any{},
			"total":  0,
			"limit":  limit,
			"offset": offset,
		})
		return
	}

	sets, total, err := s.EvalService.ListSets(limit, offset)
	if err != nil {
		writeError(w, r, NewInternal(err))
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

	writeJSON(w, http.StatusOK, map[string]any{
		"items":  out,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// CreateEvalSet handles POST /api/v1/evals.
func (s *Server) CreateEvalSet(w http.ResponseWriter, r *http.Request) {
	if s.EvalService == nil {
		writeError(w, r, NewInternal(errors.New("eval service not configured")))
		return
	}

	var body struct {
		SetID       string              `json:"set_id"`
		DomainID    string              `json:"domain_id"`
		Description string              `json:"description,omitempty"`
		Cases       []evalmodel.EvalCase `json:"cases"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, r, NewValidation(err))
		return
	}
	if body.SetID == "" || body.DomainID == "" {
		writeError(w, r, NewValidation(errors.New("set_id and domain_id are required")))
		return
	}
	_, _, ok := queries.GetDomain(s.AppState, body.DomainID)
	if !ok {
		writeError(w, r, NewDomainNotFound(body.DomainID))
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
		writeError(w, r, NewInternal(err))
		return
	}

	w.Header().Set("Location", "/api/v1/evals/"+set.ID)
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         set.ID,
		"set_id":     set.SetID,
		"domain_id":  set.DomainID,
		"created_at": queries.FormatTimeValue(set.CreatedAt),
	})
}

// GetEvalSet handles GET /api/v1/evals/{setId}.
func (s *Server) GetEvalSet(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "setId")
	if s.EvalService == nil {
		writeError(w, r, &APIError{
			Code:    "EVAL_SET_NOT_FOUND",
			Message: "eval service not configured",
		})
		return
	}

	set, err := s.EvalService.GetSet(id)
	if err != nil {
		if errors.Is(err, evalmodel.ErrEvalSetNotFound) {
			writeError(w, r, &APIError{
				Code:    "EVAL_SET_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(w, r, NewInternal(err))
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

	writeJSON(w, http.StatusOK, map[string]any{
		"id":          set.ID,
		"set_id":      set.SetID,
		"domain_id":   set.DomainID,
		"description": set.Description,
		"cases":       cases,
		"created_at":  queries.FormatTimeValue(set.CreatedAt),
		"updated_at":  queries.FormatTimeValue(set.UpdatedAt),
	})
}

// RunEval handles POST /api/v1/evals/{setId}/run.
func (s *Server) RunEval(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "setId")
	if s.EvalService == nil {
		writeError(w, r, &APIError{
			Code:    "ADAPTER_NOT_IMPLEMENTED",
			Message: "eval runner not yet implemented (target: Phase 2)",
		})
		return
	}

	set, err := s.EvalService.GetSet(id)
	if err != nil {
		if errors.Is(err, evalmodel.ErrEvalSetNotFound) {
			writeError(w, r, &APIError{
				Code:    "EVAL_SET_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(w, r, NewInternal(err))
		return
	}

	_, domain, ok := queries.GetDomain(s.AppState, set.DomainID)
	if !ok {
		writeError(w, r, NewDomainNotFound(set.DomainID))
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
		writeError(w, r, NewInternal(err))
		return
	}

	// Phase 1 skeleton: mark the run completed with a neutral score. The actual
	// eval execution loop will be implemented once the worker pipeline can run
	// sessions in batch.
	_ = s.EvalService.FinishRun(run.ID, 0.0, 0, run.TotalCases, 0, 0)

	w.Header().Set("Location", fmt.Sprintf("/api/v1/evals/%s/runs/%s", set.ID, run.ID))
	writeJSON(w, http.StatusAccepted, map[string]any{
		"run_id": run.ID,
		"state":  "running",
	})
}

// GetEvalRun handles GET /api/v1/evals/{setId}/runs/{runId}.
func (s *Server) GetEvalRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runId")
	if s.EvalService == nil {
		writeError(w, r, &APIError{
			Code:    "ADAPTER_NOT_IMPLEMENTED",
			Message: "eval runner not yet implemented (target: Phase 2)",
		})
		return
	}

	run, err := s.EvalService.GetRun(runID)
	if err != nil {
		if errors.Is(err, evalmodel.ErrEvalRunNotFound) {
			writeError(w, r, &APIError{
				Code:    "EVAL_RUN_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(w, r, NewInternal(err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
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
func (s *Server) ListAudit(w http.ResponseWriter, r *http.Request) {
	limit := intQuery(r, "limit", 50)
	offset := intQuery(r, "offset", 0)
	if limit <= 0 {
		limit = 50
	}

	if s.AuditService == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"items":  []map[string]any{},
			"total":  0,
			"limit":  limit,
			"offset": offset,
		})
		return
	}

	entries, total, err := s.AuditService.List(limit, offset)
	if err != nil {
		writeError(w, r, NewInternal(err))
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

	writeJSON(w, http.StatusOK, map[string]any{
		"items":  out,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetMetrics handles GET /api/v1/metrics.
func (s *Server) GetMetrics(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	domainFilter := q.Get("domain")
	sessions := queries.ListSessions(s.AppState)
	total := 0
	completed := 0
	failed := 0
	var totalCost float64
	var totalDurationMs uint64
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
	}
	if total > 0 {
		totalCost = 0 // Cost is not yet populated by NoopAdapter; future PR.
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"domain_id":          domainFilter,
		"total_sessions":     total,
		"completed_sessions": completed,
		"failed_sessions":    failed,
		"total_cost_usd":     totalCost,
		"avg_duration_ms":    avgDurationMs(totalDurationMs, total),
		"agent_invocations":  0,
		"tool_invocations":   0,
	})
}


// ListRuntimes handles GET /api/v1/runtimes.
func (s *Server) ListRuntimes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
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
func (s *Server) ListSecrets(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"items": []map[string]any{},
		"total": 0,
	})
}

// CreateSecret handles POST /api/v1/secrets.
func (s *Server) CreateSecret(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, &APIError{
		Code:    "ADAPTER_NOT_IMPLEMENTED",
		Message: "secret store not yet implemented (target: Phase 2)",
	})
}

// UpdateSecret handles PUT /api/v1/secrets/{id}.
func (s *Server) UpdateSecret(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, &APIError{
		Code:    "ADAPTER_NOT_IMPLEMENTED",
		Message: "secret store not yet implemented (target: Phase 2)",
	})
}

// DeleteSecret handles DELETE /api/v1/secrets/{id}.
func (s *Server) DeleteSecret(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, &APIError{
		Code:    "ADAPTER_NOT_IMPLEMENTED",
		Message: "secret store not yet implemented (target: Phase 2)",
	})
}

// ListPolicies handles GET /api/v1/policies.
func (s *Server) ListPolicies(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"items": []map[string]any{},
		"total": 0,
	})
}

// ----------------------------------------------------------------------------
// tiny helpers
// ----------------------------------------------------------------------------

func intQuery(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
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
