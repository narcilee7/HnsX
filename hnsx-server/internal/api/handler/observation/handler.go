package observation

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/hnsx-io/hnsx/server/internal/domain/observation"
)

// Handler exposes observation read endpoints for the console.
type Handler struct{ sink observation.Sink }

// New wires an observation sink into the HTTP handler.
func New(sink observation.Sink) *Handler { return &Handler{sink: sink} }

// Register mounts GET /api/workspaces/:workspace_id/issues/:issue_id/observations.
func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/workspaces/:workspace_id/issues/:issue_id")
	g.GET("/observations", h.ListByIssue)
}

func (h *Handler) ListByIssue(c *gin.Context) {
	if h.sink == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "observation sink not configured"})
		return
	}
	limit := parseIntDefault(c.Query("limit"), 200)
	items, err := h.sink.ListByIssue(c.Request.Context(), c.Param("issue_id"), limit)
	if err != nil {
		if errors.Is(err, observation.ErrObservationNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if items == nil {
		items = []*observation.Observation{}
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func parseIntDefault(s string, fb int) int {
	if s == "" {
		return fb
	}
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return fb
		}
		n = n*10 + int(r-'0')
	}
	return n
}
