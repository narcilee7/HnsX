package multica_adapter

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// GetMe handles GET /api/me.
//
// Multica expects an authenticated user identity. HarnessX maps the HnsX
// tenant context to a single default user so Multica's UI can render an
// avatar and email without extra config.
func (a *Adapter) GetMe(c *gin.Context) {
	tid := tenantFromGin(c)
	writeJSON(c, http.StatusOK, UserResponse{
		ID:    string(tid),
		Name:  string(tid),
		Email: string(tid) + "@harnessx.local",
	})
}

// ListWorkspaces handles GET /api/workspaces.
//
// HnsX has a single implicit workspace per tenant. We synthesize a workspace
// list with one entry so Multica's sidebar renders.
func (a *Adapter) ListWorkspaces(c *gin.Context) {
	tid := tenantFromGin(c)
	ws := WorkspaceResponse{
		ID:        string(tid),
		Name:      string(tid),
		Slug:      string(tid),
		Settings:  rawEmptyObject(),
		CreatedAt: nowISO(),
		UpdatedAt: nowISO(),
	}
	writeJSON(c, http.StatusOK, []WorkspaceResponse{ws})
}

// CreateWorkspace handles POST /api/workspaces.
//
// HnsX currently auto-provisions tenants; this is a thin pass-through that
// returns a workspace with a server-generated UUID so Multica's UI can flow
// forward.
func (a *Adapter) CreateWorkspace(c *gin.Context) {
	var body struct {
		Name        string `json:"name"`
		Slug        string `json:"slug"`
		Description string `json:"description"`
	}
	_ = c.ShouldBindJSON(&body)

	id := uuid.NewString()
	if body.Slug == "" {
		body.Slug = id
	}
	if body.Name == "" {
		body.Name = "Workspace"
	}
	writeJSON(c, http.StatusCreated, WorkspaceResponse{
		ID:        id,
		Name:      body.Name,
		Slug:      body.Slug,
		Description: body.Description,
		Settings:  rawEmptyObject(),
		CreatedAt: nowISO(),
		UpdatedAt: nowISO(),
	})
}

// GetWorkspace handles GET /api/workspaces/:id.
func (a *Adapter) GetWorkspace(c *gin.Context) {
	id := c.Param("id")
	writeJSON(c, http.StatusOK, WorkspaceResponse{
		ID:        id,
		Name:      id,
		Slug:      id,
		Settings:  rawEmptyObject(),
		CreatedAt: nowISO(),
		UpdatedAt: nowISO(),
	})
}

// UpdateWorkspace handles PUT/PATCH /api/workspaces/:id.
func (a *Adapter) UpdateWorkspace(c *gin.Context) {
	id := c.Param("id")
	var body map[string]any
	_ = c.ShouldBindJSON(&body)
	name, _ := body["name"].(string)
	desc, _ := body["description"].(string)
	if name == "" {
		name = id
	}
	writeJSON(c, http.StatusOK, WorkspaceResponse{
		ID:          id,
		Name:        name,
		Slug:        id,
		Description: desc,
		Settings:    rawEmptyObject(),
		CreatedAt:   nowISO(),
		UpdatedAt:   nowISO(),
	})
}

// ListMembers handles GET /api/workspaces/:id/members.
func (a *Adapter) ListMembers(c *gin.Context) {
	wid := c.Param("id")
	tid := tenantFromGin(c)
	writeJSON(c, http.StatusOK, []MemberResponse{{
		ID:          uuid.NewString(),
		WorkspaceID: wid,
		UserID:      string(tid),
		Role:        "owner",
		CreatedAt:   nowISO(),
	}})
}

// LeaveWorkspace handles POST /api/workspaces/:id/leave.
func (a *Adapter) LeaveWorkspace(c *gin.Context) {
	writeJSON(c, http.StatusOK, gin.H{"ok": true})
}
