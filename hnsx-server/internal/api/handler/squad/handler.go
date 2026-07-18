package squad

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	squaddto "github.com/hnsx-io/hnsx/server/internal/api/dto/squad"
	"github.com/hnsx-io/hnsx/server/internal/domain/squad"
	squadsvc "github.com/hnsx-io/hnsx/server/internal/service/squad"
)

type Handler struct{ svc *squadsvc.Service }

func New(s *squadsvc.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/workspaces/:workspace_id/squads")
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET(":id", h.Get)
	g.PATCH(":id", h.Update)
	g.POST(":id/members", h.AddMember)
	g.DELETE(":id/members/:member_id", h.RemoveMember)
	g.DELETE(":id", h.Delete)
}

func (h *Handler) Create(c *gin.Context) {
	var req squaddto.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in := squadsvc.CreateInput{
		WorkspaceID: c.Param("workspace_id"),
		Name:        req.Name,
		Description: req.Description,
	}
	for _, m := range req.Members {
		in.Members = append(in.Members, m.ToDomain())
	}
	got, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, squaddto.FromDomain(got))
}

func (h *Handler) List(c *gin.Context) {
	items, err := h.svc.ListByWorkspace(c.Request.Context(), c.Param("workspace_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, squaddto.FromDomainList(items))
}

func (h *Handler) Get(c *gin.Context) {
	got, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, squaddto.FromDomain(got))
}

func (h *Handler) Update(c *gin.Context) {
	var req squaddto.UpdateRequest
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
	if req.Members != nil {
		got.Members = make([]squad.Member, 0, len(req.Members))
		for _, m := range req.Members {
			got.Members = append(got.Members, m.ToDomain())
		}
	}
	if err := h.svc.Update(c.Request.Context(), got); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, squaddto.FromDomain(got))
}

func (h *Handler) AddMember(c *gin.Context) {
	var req squaddto.AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.AddMember(c.Request.Context(), c.Param("id"), req.Member.ToDomain()); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) RemoveMember(c *gin.Context) {
	if err := h.svc.RemoveMember(c.Request.Context(), c.Param("id"), c.Param("member_id")); err != nil {
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
	if errors.Is(err, squad.ErrSquadNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}