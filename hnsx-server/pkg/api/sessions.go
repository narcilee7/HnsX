package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/app/commands"
	"github.com/hnsx-io/hnsx/server/internal/app/queries"
	"github.com/hnsx-io/hnsx/server/internal/session/broadcaster"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
	"github.com/hnsx-io/hnsx/server/pkg/worker"
)

// ListSessions handles GET /api/v1/sessions.
func (s *Server) ListSessions(w http.ResponseWriter, r *http.Request) {
	items := queries.ListSessions(s.AppState, tenant.FromContext(r.Context()))
	sort.Slice(items, func(i, j int) bool {
		return items[i].StartedAt.After(items[j].StartedAt)
	})

	q := r.URL.Query()
	domainFilter := q.Get("domain")
	stateFilter := q.Get("state")

	out := make([]map[string]any, 0, len(items))
	for _, sess := range items {
		if domainFilter != "" && sess.DomainID != domainFilter {
			continue
		}
		if stateFilter != "" && sess.State != stateFilter {
			continue
		}
		out = append(out, map[string]any{
			"id":             sess.ID,
			"domain_id":      sess.DomainID,
			"domain_version": sess.DomainVersion,
			"orchestration":  sess.Orchestration,
			"state":          sess.State,
			"started_at":     queries.FormatTimeValue(sess.StartedAt),
			"completed_at":   queries.FormatTime(sess.CompletedAt),
			"summary":        sessionSummary(sess),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items":  out,
		"total":  len(out),
		"limit":  len(out),
		"offset": 0,
	})
}

// GetSession handles GET /api/v1/sessions/{id}.
func (s *Server) GetSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess, ok := queries.GetSession(s.AppState, tenant.FromContext(r.Context()), id)
	if !ok {
		writeError(w, r, NewSessionNotFound(id))
		return
	}
	writeJSON(w, http.StatusOK, jsonSession(sess))
}

// TriggerSession handles POST /api/v1/sessions.
func (s *Server) TriggerSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DomainID      string         `json:"domain_id"`
		DomainVersion string         `json:"domain_version,omitempty"`
		Trigger       map[string]any `json:"trigger,omitempty"`
	}
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, r, NewValidation(err))
		return
	}
	s.triggerSession(w, r, tenant.FromContext(r.Context()), body.DomainID, body.Trigger)
}

// triggerSession is the shared backend for /sessions and /domains/{id}/run.
func (s *Server) triggerSession(w http.ResponseWriter, r *http.Request, tenantID tenant.ID, domainID string, trigger map[string]any) {
	if domainID == "" {
		writeError(w, r, &APIError{
			Code:    "INVALID_REQUEST",
			Message: "domain_id is required",
		})
		return
	}
	_, d, ok := queries.GetDomain(s.AppState, tenantID, domainID)
	if !ok {
		writeError(w, r, NewDomainNotFound(domainID))
		return
	}

	sess, err := commands.TriggerSession(s.AppState, tenantID, d, trigger, runtime.NewSessionID)
	if err != nil {
		writeError(w, r, NewInternal(err))
		return
	}

	bc := s.AppState.AttachBroadcaster(sess.ID)

	if s.SessionQueue != nil {
		if err := s.enqueueForWorker(tenantID, sess, d, trigger); err != nil {
			writeError(w, r, NewInternal(err))
			return
		}
		w.Header().Set("Location", commands.BuildSessionLocation(sess.ID))
		writeJSON(w, http.StatusAccepted, map[string]any{
			"id":    sess.ID,
			"state": sess.State,
		})
		return
	}

	if s.Executor == nil {
		writeError(w, r, NewInternal(errors.New("executor not configured")))
		return
	}

	go s.runInBackground(tenantID, sess, d, bc, trigger)

	w.Header().Set("Location", commands.BuildSessionLocation(sess.ID))
	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":    sess.ID,
		"state": sess.State,
	})
}

// enqueueForWorker serializes the domain spec + trigger and puts the session
// on the worker queue. The worker will PullSession, run it, and stream
// observations back via the gRPC StreamChannel.
func (s *Server) enqueueForWorker(tenantID tenant.ID, sess *app.RegisteredSession, d *app.RegisteredDomain, trigger map[string]any) error {
	specJSON, err := json.Marshal(d.Spec)
	if err != nil {
		return fmt.Errorf("marshal domain spec: %w", err)
	}
	triggerJSON, err := json.Marshal(trigger)
	if err != nil {
		return fmt.Errorf("marshal trigger: %w", err)
	}

	req := &worker.SessionRequest{
		SessionID:            sess.ID,
		DomainID:             d.ID,
		DomainVersion:        d.Version,
		DomainSpecJSON:       string(specJSON),
		TriggerPayloadJSON:   string(triggerJSON),
		TraceID:              sess.ID,
		CorrelationID:        sess.ID,
		RequiredCapabilities: spec.DeriveCapabilities(d.Spec),
	}

	s.SessionQueue.Enqueue(req)

	bc := s.AppState.AttachBroadcaster(sess.ID)
	_ = bc.Publish(context.Background(), runtime.Observation{
		Kind:      "state",
		SessionID: sess.ID,
		DomainID:  d.ID,
		Payload:   map[string]any{"state": "pending", "worker_pool": true},
		Timestamp: time.Now().UTC(),
	})
	return nil
}

// runInBackground executes the registered session via the executor.
func (s *Server) runInBackground(tenantID tenant.ID, sess *app.RegisteredSession, d *app.RegisteredDomain, bc *broadcaster.Broadcaster, trigger map[string]any) {
	s.AppState.UpdateSessionState(tenantID, sess.ID, "running")
	executor := s.Executor.WithBroadcaster(bc)

	ctx := runtime.WithSessionID(context.Background(), sess.ID)

	result, err := executor.Execute(ctx, d.Spec, trigger)
	if result != nil {
		s.AppState.SetSessionResult(tenantID, sess.ID, result)
	}
	if err != nil {
		s.AppState.UpdateSessionState(tenantID, sess.ID, "failed")
	} else {
		s.AppState.UpdateSessionState(tenantID, sess.ID, "completed")
	}

	state, _ := queries.GetSession(s.AppState, tenantID, sess.ID)
	bc.Publish(ctx, runtime.Observation{
		Kind:      "state",
		SessionID: sess.ID,
		DomainID:  sess.DomainID,
		Payload:   map[string]any{"state": state.State, "result": result},
	})
}

// CancelSession handles POST /api/v1/sessions/{id}/cancel.
func (s *Server) CancelSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess, err := commands.CancelSession(s.AppState, tenant.FromContext(r.Context()), id)
	if err != nil {
		if errors.Is(err, commands.ErrSessionNotFound) {
			writeError(w, r, NewSessionNotFound(id))
			return
		}
		writeError(w, r, &APIError{
			Code:    "INVALID_REQUEST",
			Message: err.Error(),
		})
		return
	}

	if s.WorkerRegistry != nil {
		if workerID, ok := s.WorkerRegistry.SessionWorker(id); ok {
			s.WorkerRegistry.SendCancel(workerID, id, "user requested cancel", time.Now().Add(5*time.Second).UnixMilli())
		}
	}

	writeJSON(w, http.StatusOK, jsonSession(sess))
}

// RerunSession handles POST /api/v1/sessions/{id}/rerun.
func (s *Server) RerunSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	_, err := commands.RerunSession(s.AppState, tenant.FromContext(r.Context()), id, runtime.NewSessionID)
	if err != nil {
		if errors.Is(err, commands.ErrSessionNotFound) {
			writeError(w, r, NewSessionNotFound(id))
			return
		}
		writeError(w, r, NewInternal(err))
		return
	}
	// Re-trigger uses the same backend as a fresh session.
	s.triggerSession(w, r, tenant.FromContext(r.Context()), id, nil)
}

// GetSessionTrace handles GET /api/v1/sessions/{id}/trace.
//
// Returns the persisted observation trace for the session. When TraceService
// is not configured, falls back to the in-memory broadcaster replay buffer.
func (s *Server) GetSessionTrace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess, ok := queries.GetSession(s.AppState, tenant.FromContext(r.Context()), id)
	if !ok {
		writeError(w, r, NewSessionNotFound(id))
		return
	}

	observations := []map[string]any{}

	if s.TraceService != nil {
		records, err := s.TraceService.BySession(id)
		if err != nil {
			writeError(w, r, NewInternal(err))
			return
		}
		for _, rec := range records {
			observations = append(observations, map[string]any{
				"kind":              rec.Kind,
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
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"trace_id":     id,
		"session_id":   id,
		"domain_id":    sess.DomainID,
		"observations": observations,
	})
}

// StreamSessionEvents handles GET /api/v1/sessions/{id}/events (SSE).
//
// On open, it replays the broadcaster's current snapshot, then streams new
// events as they are published. Closes when the session completes or the
// client disconnects.
func (s *Server) StreamSessionEvents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := queries.GetSession(s.AppState, tenant.FromContext(r.Context()), id); !ok {
		writeError(w, r, NewSessionNotFound(id))
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, r, NewInternal(errors.New("streaming unsupported")))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	bc := s.AppState.AttachBroadcaster(id)
	ch, unsubscribe := bc.Subscribe()
	defer unsubscribe()

	writeSSE(w, flusher, "state", map[string]any{"state": "subscribed", "session_id": id})

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case obs, open := <-ch:
			if !open {
				writeSSE(w, flusher, "done", map[string]any{})
				return
			}
			writeSSE(w, flusher, "observation", obs)
		case <-ticker.C:
			writeSSE(w, flusher, "heartbeat", map[string]any{"ts": time.Now().UTC().Format(time.RFC3339)})
		}
	}
}

// writeSSE serializes payload as an SSE event and flushes.
func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", b)
	flusher.Flush()
}

// ----------------------------------------------------------------------------
// helpers
// ----------------------------------------------------------------------------

func jsonSession(sess *app.RegisteredSession) map[string]any {
	out := map[string]any{
		"id":             sess.ID,
		"domain_id":      sess.DomainID,
		"domain_version": sess.DomainVersion,
		"orchestration":  sess.Orchestration,
		"state":          sess.State,
		"trigger":        sess.Trigger,
		"started_at":     queries.FormatTimeValue(sess.StartedAt),
	}
	if sess.CompletedAt != nil {
		out["completed_at"] = queries.FormatTime(sess.CompletedAt)
	}
	if sess.Result != nil {
		out["result"] = sess.Result
		out["summary"] = sessionSummary(queries.SessionListItem{
			ID:            sess.ID,
			DomainID:      sess.DomainID,
			DomainVersion: sess.DomainVersion,
			Orchestration: sess.Orchestration,
			State:         sess.State,
			StartedAt:     sess.StartedAt,
			CompletedAt:   sess.CompletedAt,
		})
	}
	return out
}

func sessionSummary(sess queries.SessionListItem) map[string]any {
	out := map[string]any{
		"duration_ms": uint64(0),
	}
	if sess.CompletedAt != nil {
		out["duration_ms"] = uint64(sess.CompletedAt.Sub(sess.StartedAt).Milliseconds())
	}
	out["mode"] = sess.Orchestration
	out["agent_invocations"] = 0
	out["tool_invocations"] = 0
	out["total_cost_usd"] = 0.0
	return out
}

