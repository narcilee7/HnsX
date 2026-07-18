// Package handler holds the gin HTTP handlers for the workspace resource.
// Handlers are thin: parse request, call service, render response. No
// business logic, no DB access — those live in service/* and infra/db.
package workspace

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	wsdto "github.com/hnsx-io/hnsx/server/internal/api/dto/workspace"
	"github.com/hnsx-io/hnsx/server/internal/domain/workspace"
	wssvc "github.com/hnsx-io/hnsx/server/internal/service/workspace"
)

// Handler binds the gin routes for the workspace resource.
type Handler struct {
	svc *wssvc.Service
}

// New constructs a Handler.
func New(s *wssvc.Service) *Handler { return &Handler{svc: s} }

// Register mounts the routes on a gin RouterGroup.
//
//	POST   /workspaces          — create
//	GET    /workspaces          — list
//	GET    /workspaces/:id      — get by ID
//	GET    /workspaces/s/:slug  — get by slug
//	PATCH  /workspaces/:id      — update
//	POST   /workspaces/:id/archive
//	DELETE /workspaces/:id
func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/workspaces")
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET("/s/:slug", h.GetBySlug)
	// NOTE: no GET /workspaces/:id — that conflicts with
	// /workspaces/:workspace_id/issues (gin rejects two :wildcards at
	// the same path depth). Clients fetch a workspace by slug, or use
	// the list endpoint with a filter. R3.x adds /workspaces/w/:id
	// once we move nested resources off the shared path.
	g.GET("/w/:id", h.Get)
	g.PATCH("/w/:id", h.Update)
	g.POST("/w/:id/archive", h.Archive)
	g.DELETE("/w/:id", h.Delete)
}

// Create handles POST /workspaces.
func (h *Handler) Create(c *gin.Context) {
	var req wsdto.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in := wssvc.CreateInput{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: req.Description,
		Context:     req.Context,
		Settings:    req.Settings,
	}
	w, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, wsdto.FromDomain(w))
}

// List handles GET /workspaces?status=&limit=&offset=.
func (h *Handler) List(c *gin.Context) {
	f := workspace.ListFilter{
		Status: workspace.Status(c.Query("status")),
		Limit:  parseIntDefault(c.Query("limit"), 50),
		Offset: parseIntDefault(c.Query("offset"), 0),
	}
	ws, err := h.svc.List(c.Request.Context(), f)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, wsdto.FromDomainList(ws))
}

// GetBySlug handles GET /workspaces/s/:slug.
func (h *Handler) GetBySlug(c *gin.Context) {
	w, err := h.svc.GetBySlug(c.Request.Context(), c.Param("slug"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, wsdto.FromDomain(w))
}

// Get handles GET /workspaces/:id.
func (h *Handler) Get(c *gin.Context) {
	w, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, wsdto.FromDomain(w))
}

// Update handles PATCH /workspaces/:id.
func (h *Handler) Update(c *gin.Context) {
	var req wsdto.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	w, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	if req.Name != nil {
		w.Name = *req.Name
	}
	if req.Description != nil {
		w.Description = *req.Description
	}
	if req.Context != nil {
		w.Context = *req.Context
	}
	if req.Settings != nil {
		w.Settings = *req.Settings
	}
	if err := h.svc.Update(c.Request.Context(), w); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, wsdto.FromDomain(w))
}

// Archive handles POST /workspaces/:id/archive.
func (h *Handler) Archive(c *gin.Context) {
	if err := h.svc.Archive(c.Request.Context(), c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Delete handles DELETE /workspaces/:id.
func (h *Handler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// writeError maps service-layer errors to HTTP statuses.
func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, workspace.ErrWorkspaceNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func parseIntDefault(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return fallback
		}
		n = n*10 + int(r-'0')
	}
	return n
}