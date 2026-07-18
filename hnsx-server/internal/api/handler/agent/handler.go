package agent

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	agentdto "github.com/hnsx-io/hnsx/server/internal/api/dto/agent"
	"github.com/hnsx-io/hnsx/server/internal/domain/agent"
	agentsvc "github.com/hnsx-io/hnsx/server/internal/service/agent"
)

type Handler struct{ svc *agentsvc.Service }

func New(s *agentsvc.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/workspaces/:workspace_id/agents")
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET(":id", h.Get)
	g.PATCH(":id", h.Update)
	g.POST(":id/archive", h.Archive)
	g.DELETE(":id", h.Delete)
}

func (h *Handler) Create(c *gin.Context) {
	var req agentdto.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in := agentsvc.CreateInput{
		WorkspaceID:        c.Param("workspace_id"),
		Name:               req.Name,
		Description:        req.Description,
		AvatarURL:          req.AvatarURL,
		RuntimeMode:        agent.RuntimeMode(orDefault(req.RuntimeMode, string(agent.RuntimeLocal))),
		Visibility:         agent.Visibility(orDefault(req.Visibility, string(agent.VisibilityWorkspace))),
		MaxConcurrentTasks: req.MaxConcurrentTasks,
		OwnerID:            req.OwnerID,
		RuntimeConfig:      req.RuntimeConfig,
	}
	got, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, agentdto.FromDomain(got))
}

func (h *Handler) List(c *gin.Context) {
	f := agent.ListFilter{
		Status:     agent.Status(c.Query("status")),
		Visibility: agent.Visibility(c.Query("visibility")),
		Limit:      parseIntDefault(c.Query("limit"), 50),
		Offset:     parseIntDefault(c.Query("offset"), 0),
	}
	items, err := h.svc.ListByWorkspace(c.Request.Context(), c.Param("workspace_id"), f)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, agentdto.FromDomainList(items))
}

func (h *Handler) Get(c *gin.Context) {
	got, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, agentdto.FromDomain(got))
}

func (h *Handler) Update(c *gin.Context) {
	var req agentdto.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	got, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	if req.Name != nil {
		got.Name = *req.Name
	}
	if req.Description != nil {
		got.Description = *req.Description
	}
	if req.AvatarURL != nil {
		got.AvatarURL = req.AvatarURL
	}
	if req.RuntimeMode != nil {
		got.RuntimeMode = agent.RuntimeMode(*req.RuntimeMode)
	}
	if req.RuntimeConfig != nil {
		got.RuntimeConfig = *req.RuntimeConfig
	}
	if req.Visibility != nil {
		got.Visibility = agent.Visibility(*req.Visibility)
	}
	if req.MaxConcurrentTasks != nil {
		got.MaxConcurrentTasks = *req.MaxConcurrentTasks
	}
	if err := h.svc.Update(c.Request.Context(), got); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, agentdto.FromDomain(got))
}

func (h *Handler) Archive(c *gin.Context) {
	if err := h.svc.Archive(c.Request.Context(), c.Param("id")); err != nil {
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
	if errors.Is(err, agent.ErrAgentNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

func orDefault(s, fb string) string {
	if s == "" {
		return fb
	}
	return s
}