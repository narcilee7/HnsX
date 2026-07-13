package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/handler"
	"github.com/hnsx-io/hnsx/server/pkg/handler/viewmodel"
)

// ListSessions handles GET /api/v1/sessions.
func (s *Server) ListSessions(c *gin.Context) {
	out, err := s.Handlers.ListSessions(c.Request.Context(), handler.ListSessionsInput{
		TenantID: tenantFromGin(c),
		Filters: viewmodel.SessionFilters{
			DomainID: c.Query("domain"),
			State:    c.Query("state"),
		},
	})
	if err != nil {
		writeError(c, mapSessionError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Sessions)
}

// GetSession handles GET /api/v1/sessions/:id.
func (s *Server) GetSession(c *gin.Context) {
	out, err := s.Handlers.GetSession(c.Request.Context(), handler.GetSessionInput{
		TenantID:  tenantFromGin(c),
		SessionID: c.Param("id"),
	})
	if err != nil {
		writeError(c, mapSessionError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Session)
}

// TriggerSession handles POST /api/v1/sessions.
func (s *Server) TriggerSession(c *gin.Context) {
	var body struct {
		DomainID      string         `json:"domain_id"`
		DomainVersion string         `json:"domain_version,omitempty"`
		Trigger       map[string]any `json:"trigger,omitempty"`
	}
	if err := decodeJSONBody(c, &body); err != nil {
		writeError(c, NewValidation(err))
		return
	}
	s.triggerSession(c, tenantFromGin(c), body.DomainID, body.Trigger)
}

// triggerSession is the shared backend for /sessions and /domains/:id/run.
func (s *Server) triggerSession(c *gin.Context, tenantID tenant.ID, domainID string, trigger map[string]any) {
	if domainID == "" {
		writeError(c, NewInvalidRequest("domain_id is required"))
		return
	}
	out, err := s.Handlers.TriggerSession(c.Request.Context(), handler.TriggerSessionInput{
		TenantID:      tenantID,
		DomainID:      domainID,
		DomainVersion: "",
		Trigger:       trigger,
	})
	if err != nil {
		if handler.IsDomainNotFound(err) {
			writeError(c, NewDomainNotFound(domainID))
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	c.Header("Location", out.Location)
	writeJSON(c, http.StatusAccepted, out.Session)
}

// CancelSession handles POST /api/v1/sessions/:id/cancel.
func (s *Server) CancelSession(c *gin.Context) {
	id := c.Param("id")
	out, err := s.Handlers.CancelSession(c.Request.Context(), handler.CancelSessionInput{
		TenantID:  tenantFromGin(c),
		SessionID: id,
	})
	if err != nil {
		writeError(c, mapSessionError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Session)
}

// RerunSession handles POST /api/v1/sessions/:id/rerun.
func (s *Server) RerunSession(c *gin.Context) {
	id := c.Param("id")
	out, err := s.Handlers.RerunSession(c.Request.Context(), handler.RerunSessionInput{
		TenantID:  tenantFromGin(c),
		SessionID: id,
	})
	if err != nil {
		writeError(c, mapSessionError(err))
		return
	}
	c.Header("Location", out.Location)
	writeJSON(c, http.StatusAccepted, out.Session)
}

// GetSessionTrace handles GET /api/v1/sessions/:id/trace.
func (s *Server) GetSessionTrace(c *gin.Context) {
	id := c.Param("id")
	out, err := s.Handlers.GetSessionTrace(c.Request.Context(), handler.GetSessionTraceInput{
		TenantID:  tenantFromGin(c),
		SessionID: id,
	})
	if err != nil {
		writeError(c, mapSessionError(err))
		return
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"trace_id":     out.TraceID,
		"session_id":   out.SessionID,
		"domain_id":    out.DomainID,
		"observations": out.Observations,
	})
}

// StreamSessionEvents handles GET /api/v1/sessions/:id/events (SSE).
func (s *Server) StreamSessionEvents(c *gin.Context) {
	id := c.Param("id")
	if _, err := s.Handlers.GetSession(c.Request.Context(), handler.GetSessionInput{
		TenantID:  tenantFromGin(c),
		SessionID: id,
	}); err != nil {
		writeError(c, mapSessionError(err))
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	bc := s.AppState.AttachBroadcaster(id)
	ch, unsubscribe := bc.Subscribe()
	defer unsubscribe()

	writeSSE(c, "state", map[string]any{"state": "subscribed", "session_id": id})

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case obs, open := <-ch:
			if !open {
				writeSSE(c, "done", map[string]any{})
				return
			}
			writeSSE(c, "observation", obs)
		case <-ticker.C:
			writeSSE(c, "heartbeat", map[string]any{"ts": time.Now().UTC().Format(time.RFC3339)})
		}
	}
}

// writeSSE serializes payload as an SSE event and flushes.
func writeSSE(c *gin.Context, event string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(c.Writer, "event: %s\n", event)
	fmt.Fprintf(c.Writer, "data: %s\n\n", b)
	c.Writer.Flush()
}

// mapSessionError maps handler/session errors to canonical APIError values.
func mapSessionError(err error) *APIError {
	if err == nil {
		return nil
	}
	if handler.IsSessionNotFound(err) {
		return NewSessionNotFound("")
	}
	if handler.IsInvalidSession(err) || handler.IsAlreadyTerminal(err) {
		return &APIError{Code: "INVALID_REQUEST", Message: err.Error()}
	}
	return NewInternal(err)
}
