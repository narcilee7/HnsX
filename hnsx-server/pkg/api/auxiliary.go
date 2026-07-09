package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// ListTraces handles GET /api/v1/traces — returns the per-domain trace index.
//
// Phase 1 derives the index from registered sessions. A real implementation
// would source from Tempo.
func (s *Server) ListTraces(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	domainFilter := q.Get("domain")
	limit := intQuery(r, "limit", 50)
	offset := intQuery(r, "offset", 0)

	items := s.listSessionItems()
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
			"started_at":     sess.StartedAt.Format(time.RFC3339),
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
	sess, ok := s.lookupSession(id)
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
		"started_at":    sess.StartedAt.Format(time.RFC3339),
		"duration_ms":   durationMs(sess),
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
	writeJSON(w, http.StatusOK, map[string]any{
		"items": []map[string]any{},
		"total": 0,
	})
}

// GetEvalSet handles GET /api/v1/evals/{setId}.
func (s *Server) GetEvalSet(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, &APIError{
		Code:    "EVAL_SET_NOT_FOUND",
		Message: "eval subsystem not yet implemented (target: Phase 2)",
	})
}

// RunEval handles POST /api/v1/evals/{setId}/run.
func (s *Server) RunEval(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, &APIError{
		Code:    "ADAPTER_NOT_IMPLEMENTED",
		Message: "eval runner not yet implemented (target: Phase 2)",
	})
}

// GetEvalRun handles GET /api/v1/evals/{setId}/runs/{runId}.
func (s *Server) GetEvalRun(w http.ResponseWriter, r *http.Request) {
	writeError(w, r, &APIError{
		Code:    "ADAPTER_NOT_IMPLEMENTED",
		Message: "eval runner not yet implemented (target: Phase 2)",
	})
}

// ListAudit handles GET /api/v1/audit.
func (s *Server) ListAudit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"items": []map[string]any{},
		"total": 0,
	})
}

// GetMetrics handles GET /api/v1/metrics.
func (s *Server) GetMetrics(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	domainFilter := q.Get("domain")
	sessions := s.listSessionItems()
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

func durationMs(sess *registeredSession) uint64 {
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
