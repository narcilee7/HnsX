package policy

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	pdto "github.com/hnsx-io/hnsx/server/internal/api/dto/policy"
	"github.com/hnsx-io/hnsx/server/internal/domain/policy"
	psvc "github.com/hnsx-io/hnsx/server/internal/service/policy"
)

type Handler struct{ svc *psvc.Service }

func New(s *psvc.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/workspaces/:workspace_id/policies")
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET(":id", h.Get)
	g.PATCH(":id", h.Update)
	g.DELETE(":id", h.Delete)
}

func (h *Handler) Create(c *gin.Context) {
	var req pdto.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in := psvc.CreateInput{
		WorkspaceID: c.Param("workspace_id"),
		Name:        req.Name,
		Description: req.Description,
	}
	for _, r := range req.Rules {
		in.Rules = append(in.Rules, policy.Rule{
			ID:         r.ID,
			Kind:       policy.Kind(r.Kind),
			Expression: r.Expression,
			Action:     policy.Action(r.Action),
			Message:    r.Message,
			Priority:   r.Priority,
		})
	}
	got, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, pdto.FromDomain(got))
}

func (h *Handler) List(c *gin.Context) {
	items, err := h.svc.ListByWorkspace(c.Request.Context(), c.Param("workspace_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, pdto.FromDomainList(items))
}

func (h *Handler) Get(c *gin.Context) {
	got, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, pdto.FromDomain(got))
}

func (h *Handler) Update(c *gin.Context) {
	var req pdto.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	got, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	if req.Name != "" {
		got.Name = req.Name
	}
	if req.Description != "" {
		got.Description = req.Description
	}
	if len(req.Rules) > 0 {
		for _, r := range req.Rules {
			got.Rules = append(got.Rules, []byte(fmt.Sprintf(`{"id":"%s","kind":"%s","expression":%q,"action":"%s","message":%q,"priority":%d},`,
				r.ID, r.Kind, r.Expression, r.Action, r.Message, r.Priority))...)
		}
	}
	if err := h.svc.Update(c.Request.Context(), got); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, pdto.FromDomain(got))
}

func (h *Handler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeError(c *gin.Context, err error) {
	if errors.Is(err, policy.ErrPolicyNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}