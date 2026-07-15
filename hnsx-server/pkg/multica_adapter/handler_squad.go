package multica_adapter

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// squadToResponse converts an HnsX DomainSpec (mode=supervisor) into a Multica
// SquadResponse. The leader is read from the harness.session.agent field.
//
// Multica's "squad" carries a leader + members; HnsX's supervisor session is
// mode=supervisor with one "agent" field for the leader. Members are tracked
// via the workflow steps (P0) and a dedicated registry (P1).
func squadToResponse(d *domain.DomainSpec, wsID string) SquadResponse {
	leaderID := ""
	if d != nil {
		leaderID = d.Harness.Session.Agent
		if leaderID == "" {
			// Fall back to the first agent declared in the harness.
			for _, a := range d.Harness.Agents {
				leaderID = a.ID
				break
			}
		}
	}

	preview := []SquadMemberPreviewResponse{}
	if d != nil {
		for _, a := range d.Harness.Agents {
			if a.ID == leaderID {
				continue
			}
			preview = append(preview, SquadMemberPreviewResponse{
				MemberType: "agent",
				MemberID:   a.ID,
			})
		}
	}

	return SquadResponse{
		ID:            d.ID,
		WorkspaceID:   wsID,
		Name:          d.ID,
		Description:   d.Description,
		LeaderID:      leaderID,
		CreatorID:     string(tenant.DefaultID),
		CreatedAt:     nowISO(),
		UpdatedAt:     nowISO(),
		MemberCount:   len(d.Harness.Agents),
		MemberPreview: preview,
	}
}

// ListSquads handles GET /api/squads.
func (a *Adapter) ListSquads(c *gin.Context) {
	tid := tenantFromGin(c)
	wsID := c.Query("workspace_id")
	if wsID == "" {
		wsID = string(tid)
	}

	domains, err := a.app.DomainService.List(tid)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	out := make([]SquadResponse, 0)
	for i := range domains {
		d := domains[i].Spec
		if d == nil {
			continue
		}
		if d.Harness.Session.Mode != "supervisor" && d.Harness.Session.Mode != "hierarchical" {
			continue
		}
		out = append(out, squadToResponse(d, wsID))
	}
	writeJSON(c, http.StatusOK, out)
}

// CreateSquad handles POST /api/squads.
//
// Multica body: { name, description, leader_id }. We synthesize a DomainSpec
// with mode=supervisor and the leader_id stamped on the first agent + the
// session.agent field.
func (a *Adapter) CreateSquad(c *gin.Context) {
	tid := tenantFromGin(c)

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		LeaderID    string `json:"leader_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errorJSON(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if body.Name == "" {
		errorJSON(c, http.StatusBadRequest, "NAME_REQUIRED", "name is required")
		return
	}
	if body.LeaderID == "" {
		errorJSON(c, http.StatusBadRequest, "LEADER_REQUIRED", "leader_id is required")
		return
	}

	id := uuid.NewString()
	spec := &domain.DomainSpec{
		ID:          id,
		Version:     "0.1.0",
		Description: body.Description,
		Harness: domain.HarnessSpec{
			Agents: map[string]domain.AgentSpec{
				body.LeaderID: {
					ID:       body.LeaderID,
					Provider: "anthropic",
					Model:    "claude-haiku-4-5",
					Adapter:  domain.AdapterConfig{Kind: "anthropic", TimeoutSeconds: 60},
				},
			},
			Session: domain.SessionSpec{
				Mode:  domain.Supervisor,
				Agent: body.LeaderID,
			},
		},
	}
	if _, err := a.app.DomainService.Register(tid, spec); err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	writeJSON(c, http.StatusCreated, squadToResponse(spec, string(tid)))
}

// GetSquad handles GET /api/squads/:id.
func (a *Adapter) GetSquad(c *gin.Context) {
	tid := tenantFromGin(c)
	id := c.Param("id")
	rd, err := a.app.DomainService.Get(tid, id)
	if err != nil || rd == nil || rd.Spec == nil {
		errorJSON(c, http.StatusNotFound, "SQUAD_NOT_FOUND", "squad not found: "+id)
		return
	}
	writeJSON(c, http.StatusOK, squadToResponse(rd.Spec, string(tid)))
}

// UpdateSquad handles PATCH /api/squads/:id.
func (a *Adapter) UpdateSquad(c *gin.Context) {
	tid := tenantFromGin(c)
	id := c.Param("id")
	rd, err := a.app.DomainService.Get(tid, id)
	if err != nil || rd == nil || rd.Spec == nil {
		errorJSON(c, http.StatusNotFound, "SQUAD_NOT_FOUND", "squad not found: "+id)
		return
	}
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Instructions string `json:"instructions"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Description != "" {
		rd.Spec.Description = body.Description
	}
	writeJSON(c, http.StatusOK, squadToResponse(rd.Spec, string(tid)))
}

// DeleteSquad handles DELETE /api/squads/:id.
func (a *Adapter) DeleteSquad(c *gin.Context) {
	tid := tenantFromGin(c)
	id := c.Param("id")
	if err := a.app.DomainService.Delete(tid, id); err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

// ListSquadMembers handles GET /api/squads/:id/members.
func (a *Adapter) ListSquadMembers(c *gin.Context) {
	tid := tenantFromGin(c)
	id := c.Param("id")
	rd, err := a.app.DomainService.Get(tid, id)
	if err != nil || rd == nil || rd.Spec == nil {
		writeJSON(c, http.StatusOK, []any{})
		return
	}
	out := make([]map[string]any, 0, len(rd.Spec.Harness.Agents))
	for _, a := range rd.Spec.Harness.Agents {
		out = append(out, map[string]any{
			"squad_id":    id,
			"member_type": "agent",
			"member_id":   a.ID,
		})
	}
	writeJSON(c, http.StatusOK, out)
}

// AddSquadMember handles POST /api/squads/:id/members.
func (a *Adapter) AddSquadMember(c *gin.Context) {
	notImplemented(c, "AddSquadMember")
}
