package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Health is the GET /healthz handler — process is alive.
func (s *Server) Health(c *gin.Context) {
	writeJSON(c, http.StatusOK, map[string]any{
		"status": "ok",
		"build":  s.BuildInfo,
	})
}

// Readiness is the GET /readyz handler — DB is reachable (when configured).
func (s *Server) Readiness(c *gin.Context) {
	type check struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Error  string `json:"error,omitempty"`
	}

	checks := []check{
		{Name: "process", Status: "ok"},
	}

	if s.DB != nil && !s.DB.IsNoDB() {
		probeCtx, cancel := s.timeoutCtx(c.Request)
		defer cancel()
		if err := s.DB.SQL.PingContext(probeCtx); err != nil {
			checks = append(checks, check{Name: "database", Status: "down", Error: err.Error()})
			writeJSON(c, http.StatusServiceUnavailable, map[string]any{
				"status": "not_ready",
				"checks": checks,
			})
			return
		}
		checks = append(checks, check{Name: "database", Status: "ok"})
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"status": "ready",
		"checks": checks,
	})
}
