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

	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/internal/session/broadcaster"
	"github.com/hnsx-io/hnsx/server/pkg/worker"
)

// ListSessions handles GET /api/v1/sessions.
func (s *Server) ListSessions(w http.ResponseWriter, r *http.Request) {
	items := s.listSessionItems()
	sort.Slice(items, func(i, j int) bool {
		return items[i].StartedAt.After(items[j].StartedAt)
	})

	// Optional filters via ?domain=&state=
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
			"started_at":     sess.StartedAt.Format(time.RFC3339),
			"completed_at":   formatCompletedAt(sess.CompletedAt),
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
	sess, ok := s.lookupSession(id)
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
	s.triggerSession(w, r, body.DomainID, body.Trigger)
}

// triggerSession is the shared backend for /sessions and /domains/{id}/run.
func (s *Server) triggerSession(w http.ResponseWriter, r *http.Request, domainID string, trigger map[string]any) {
	if domainID == "" {
		writeError(w, r, &APIError{
			Code:    "INVALID_REQUEST",
			Message: "domain_id is required",
		})
		return
	}
	d, ok := s.lookupDomain(domainID)
	if !ok {
		writeError(w, r, NewDomainNotFound(domainID))
		return
	}

	sess := &registeredSession{
		ID:            runtime.NewSessionID(domainID),
		DomainID:      d.ID,
		DomainVersion: d.Version,
		Orchestration: d.Spec.Harness.Session.Mode,
		State:         "pending",
		Trigger:       trigger,
		StartedAt:     time.Now().UTC(),
	}
	s.registerSession(sess)

	bc := s.attachBroadcaster(sess.ID)

	// V1.1: if a worker pool is wired, enqueue the session for a Python
	// worker. Otherwise fall back to the in-process Go executor.
	if s.SessionQueue != nil {
		if err := s.enqueueForWorker(sess, d, trigger); err != nil {
			writeError(w, r, NewInternal(err))
			return
		}
		w.Header().Set("Location", "/api/v1/sessions/"+sess.ID)
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

	// Execute synchronously for Phase 1; an async variant lands in a
	// follow-up PR (the API surface stays the same).
	go s.runInBackground(sess, d, bc, trigger)

	w.Header().Set("Location", "/api/v1/sessions/"+sess.ID)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"id":    sess.ID,
		"state": sess.State,
	})
}

// enqueueForWorker serializes the domain spec + trigger and puts the session
// on the worker queue. The worker will PullSession, run it, and stream
// observations back via the gRPC StreamChannel.
func (s *Server) enqueueForWorker(sess *registeredSession, d *registeredDomain, trigger map[string]any) error {
	specJSON, err := json.Marshal(d.Spec)
	if err != nil {
		return fmt.Errorf("marshal domain spec: %w", err)
	}
	triggerJSON, err := json.Marshal(trigger)
	if err != nil {
		return fmt.Errorf("marshal trigger: %w", err)
	}

	req := &worker.SessionRequest{
		SessionID:          sess.ID,
		DomainID:           d.ID,
		DomainVersion:      d.Version,
		DomainSpecJSON:     string(specJSON),
		TriggerPayloadJSON: string(triggerJSON),
		TraceID:            sess.ID,
		CorrelationID:      sess.ID,
	}

	s.SessionQueue.Enqueue(req)

	bc := s.attachBroadcaster(sess.ID)
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
func (s *Server) runInBackground(sess *registeredSession, d *registeredDomain, bc *broadcaster.Broadcaster, trigger map[string]any) {
	sess.State = "running"
	executor := s.Executor.WithBroadcaster(bc)

	ctx := runtime.WithSessionID(context.Background(), sess.ID)

	result, err := executor.Execute(ctx, d.Spec, trigger)
	if result != nil {
		sess.Result = result
	}
	if err != nil {
		sess.State = "failed"
	} else {
		sess.State = "completed"
	}
	done := time.Now().UTC()
	sess.CompletedAt = &done

	bc.Publish(ctx, runtime.Observation{
		Kind:      "state",
		SessionID: sess.ID,
		DomainID:  sess.DomainID,
		Payload:   map[string]any{"state": sess.State, "result": result},
	})
}

// CancelSession handles POST /api/v1/sessions/{id}/cancel.
func (s *Server) CancelSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	sess, ok := s.lookupSession(id)
	if !ok {
		writeError(w, r, NewSessionNotFound(id))
		return
	}
	if sess.State == "completed" || sess.State == "failed" {
		writeError(w, r, &APIError{
			Code:    "INVALID_REQUEST",
			Message: fmt.Sprintf("session is already in terminal state %q", sess.State),
		})
		return
	}

	// V1.1: if a worker pool is wired, ask the assigned worker to cancel.
	if s.WorkerRegistry != nil {
		if workerID, ok := s.WorkerRegistry.SessionWorker(id); ok {
			s.WorkerRegistry.SendCancel(workerID, id, "user requested cancel", time.Now().Add(5*time.Second).UnixMilli())
		}
	}

	sess.State = "canceled"
	done := time.Now().UTC()
	sess.CompletedAt = &done
	s.detachBroadcaster(sess.ID)
	writeJSON(w, http.StatusOK, jsonSession(sess))
}

// RerunSession handles POST /api/v1/sessions/{id}/rerun.
func (s *Server) RerunSession(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	prev, ok := s.lookupSession(id)
	if !ok {
		writeError(w, r, NewSessionNotFound(id))
		return
	}
	s.triggerSession(w, r, prev.DomainID, prev.Trigger)
}

// GetSessionTrace handles GET /api/v1/sessions/{id}/trace.
//
// Phase 1 returns a summary envelope pointing at the SSE replay endpoint.
func (s *Server) GetSessionTrace(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := s.lookupSession(id); !ok {
		writeError(w, r, NewSessionNotFound(id))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"trace_id":   id,
		"session_id": id,
		"replay":     "/api/v1/sessions/" + id + "/events",
	})
}

// StreamSessionEvents handles GET /api/v1/sessions/{id}/events (SSE).
//
// On open, it replays the broadcaster's current snapshot, then streams new
// events as they are published. Closes when the session completes or the
// client disconnects.
func (s *Server) StreamSessionEvents(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := s.lookupSession(id); !ok {
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

	// Subscribe to broadcaster.
	bc := s.attachBroadcaster(id)
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

func jsonSession(sess *registeredSession) map[string]any {
	out := map[string]any{
		"id":             sess.ID,
		"domain_id":      sess.DomainID,
		"domain_version": sess.DomainVersion,
		"orchestration":  sess.Orchestration,
		"state":          sess.State,
		"trigger":        sess.Trigger,
		"started_at":     sess.StartedAt.Format(time.RFC3339),
	}
	if sess.CompletedAt != nil {
		out["completed_at"] = sess.CompletedAt.Format(time.RFC3339)
	}
	if sess.Result != nil {
		out["result"] = sess.Result
		out["summary"] = sessionSummary(sess)
	}
	return out
}

func sessionSummary(sess *registeredSession) map[string]any {
	out := map[string]any{
		"duration_ms": uint64(0),
	}
	if sess.CompletedAt != nil {
		out["duration_ms"] = uint64(sess.CompletedAt.Sub(sess.StartedAt).Milliseconds())
	}
	if sess.Result != nil {
		out["mode"] = sess.Result.Mode
		out["agent_invocations"] = 0
		out["tool_invocations"] = 0
		out["total_cost_usd"] = 0.0
	}
	return out
}

func formatCompletedAt(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}
