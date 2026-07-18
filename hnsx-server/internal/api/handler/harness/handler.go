package harness

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	hdto "github.com/hnsx-io/hnsx/server/internal/api/dto/harness"
	"github.com/hnsx-io/hnsx/server/internal/domain/harness"
	hsvc "github.com/hnsx-io/hnsx/server/internal/service/harness"
)

type Handler struct{ svc *hsvc.Service }

func New(s *hsvc.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/workspaces/:workspace_id/harnesses")
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET(":id", h.Get)
	g.PATCH(":id", h.Update)
	g.DELETE(":id", h.Delete)
}

func (h *Handler) Create(c *gin.Context) {
	var req hdto.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in := hsvc.CreateInput{
		WorkspaceID: c.Param("workspace_id"),
		Name:        req.Name,
		Description: req.Description,
		PolicyID:    req.PolicyID,
		EvalSetID:   req.EvalSetID,
		Version:     req.Version,
	}
	for _, p := range req.Prompts {
		in.Prompts = append(in.Prompts, harness.Prompt(p))
	}
	for _, s := range req.Skills {
		in.Skills = append(in.Skills, harness.SkillRef(s))
	}
	for _, t := range req.Tools {
		in.Tools = append(in.Tools, harness.ToolRef(t))
	}
	got, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, hdto.FromDomain(got))
}

func (h *Handler) List(c *gin.Context) {
	items, err := h.svc.ListByWorkspace(c.Request.Context(), c.Param("workspace_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, hdto.FromDomainList(items))
}

func (h *Handler) Get(c *gin.Context) {
	got, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, hdto.FromDomain(got))
}

func (h *Handler) Update(c *gin.Context) {
	var req hdto.UpdateRequest
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
	if req.PolicyID != nil {
		got.PolicyID = req.PolicyID
	}
	if req.EvalSetID != nil {
		got.EvalSetID = req.EvalSetID
	}
	if err := h.svc.Update(c.Request.Context(), got); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, hdto.FromDomain(got))
}

func (h *Handler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeError(c *gin.Context, err error) {
	if errors.Is(err, harness.ErrHarnessNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}