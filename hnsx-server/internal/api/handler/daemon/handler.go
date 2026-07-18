package daemon

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	daemondto "github.com/hnsx-io/hnsx/server/internal/api/dto/daemon"
	"github.com/hnsx-io/hnsx/server/internal/domain/daemon"
	daemonsvc "github.com/hnsx-io/hnsx/server/internal/service/daemon"
)

type Handler struct{ svc *daemonsvc.Service }

func New(s *daemonsvc.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/workspaces/:workspace_id/daemons")
	g.POST("", h.RegisterD)
	g.GET("", h.List)
	g.POST(":id/heartbeat", h.Heartbeat)
	g.GET(":id", h.Get)
	g.DELETE(":id", h.Delete)
}

func (h *Handler) RegisterD(c *gin.Context) {
	var req daemondto.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in := daemonsvc.RegisterInput{
		WorkspaceID: c.Param("workspace_id"),
		Name:        req.Name,
		Platform:    req.Platform,
		OS:          req.OS,
		Version:     req.Version,
	}
	got, err := h.svc.Register(c.Request.Context(), in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, daemondto.FromDomain(got))
}

func (h *Handler) List(c *gin.Context) {
	items, err := h.svc.ListByWorkspace(c.Request.Context(), c.Param("workspace_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, daemondto.FromDomainList(items))
}

func (h *Handler) Get(c *gin.Context) {
	got, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, daemondto.FromDomain(got))
}

func (h *Handler) Heartbeat(c *gin.Context) {
	if err := h.svc.Heartbeat(c.Request.Context(), c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeError(c *gin.Context, err error) {
	if errors.Is(err, daemon.ErrDaemonNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}