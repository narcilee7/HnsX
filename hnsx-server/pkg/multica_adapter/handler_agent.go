package multica_adapter

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// agentToResponse converts an HnsX DomainSpec into a Multica AgentResponse.
//
// Multica's "agent" is a single-purpose virtual teammate; HnsX stores the
// same shape as a Domain whose harness.session.mode == "single". We pick
// the first agent declared in the harness as the canonical identity.
func agentToResponse(d *domain.DomainSpec, wsID string) AgentResponse {
	name := ""
	provider := ""
	model := ""
	if d != nil && len(d.Harness.Agents) > 0 {
		// First agent wins (P0: agents is a map[string]AgentSpec).
		for _, a := range d.Harness.Agents {
			name = a.ID
			provider = a.Provider
			model = a.Model
			break
		}
	}
	if name == "" {
		name = d.ID
	}

	runtimeConfig := map[string]any{
		"provider":              provider,
		"model":                 model,
		"hnsx_domain_id":        d.ID,
		"hnsx_domain_version":   d.Version,
	}
	rcJSON, _ := json.Marshal(runtimeConfig)

	return AgentResponse{
		ID:                 d.ID,
		WorkspaceID:        wsID,
		Name:               name,
		RuntimeMode:        "local",
		RuntimeConfig:      rcJSON,
		Visibility:         "workspace",
		Status:             "idle",
		MaxConcurrentTasks: 1,
		CreatedAt:          nowISO(),
		UpdatedAt:          nowISO(),
	}
}

// ListAgents handles GET /api/workspaces/:id/agents.
func (a *Adapter) ListAgents(c *gin.Context) {
	wsID := c.Param("id")
	tid := tenantFromGin(c)

	domains, err := a.app.DomainService.List(tid)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	out := make([]AgentResponse, 0, len(domains))
	for i := range domains {
		d := domains[i].Spec
		if d == nil {
			continue
		}
		if d.Harness.Session.Mode != "single" && d.Harness.Session.Mode != "multi-turn" {
			continue
		}
		out = append(out, agentToResponse(d, wsID))
	}
	writeJSON(c, http.StatusOK, out)
}

// GetAgent handles GET /api/workspaces/:id/agents/:agentId.
func (a *Adapter) GetAgent(c *gin.Context) {
	wsID := c.Param("id")
	tid := tenantFromGin(c)
	agentID := c.Param("agentId")

	rd, err := a.app.DomainService.Get(tid, agentID)
	if err != nil || rd == nil || rd.Spec == nil {
		errorJSON(c, http.StatusNotFound, "AGENT_NOT_FOUND", "agent not found: "+agentID)
		return
	}
	writeJSON(c, http.StatusOK, agentToResponse(rd.Spec, wsID))
}

// CreateAgent handles POST /api/workspaces/:id/agents.
func (a *Adapter) CreateAgent(c *gin.Context) {
	wsID := c.Param("id")
	tid := tenantFromGin(c)

	var body struct {
		Name          string          `json:"name"`
		RuntimeConfig json.RawMessage `json:"runtime_config"`
		Visibility    string          `json:"visibility"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errorJSON(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if body.Name == "" {
		body.Name = uuid.NewString()
	}

	// Parse runtime_config to extract provider/model.
	var rc map[string]any
	if len(body.RuntimeConfig) > 0 {
		_ = json.Unmarshal(body.RuntimeConfig, &rc)
	}
	provider, _ := rc["provider"].(string)
	if provider == "" {
		provider = "anthropic"
	}
	model, _ := rc["model"].(string)
	if model == "" {
		model = "claude-haiku-4-5"
	}

	id := uuid.NewString()
	spec := &domain.DomainSpec{
		ID:          id,
		Version:     "0.1.0",
		Description: "agent created via multica_adapter",
		Harness: domain.HarnessSpec{
			Agents: map[string]domain.AgentSpec{
				id: {
					ID:       id,
					Provider: provider,
					Model:    model,
					Adapter:  domain.AdapterConfig{Kind: provider, TimeoutSeconds: 60},
				},
			},
			Session: domain.SessionSpec{Mode: domain.Single},
		},
	}
	if _, err := a.app.DomainService.Register(tid, spec); err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if a.api != nil {
		_ = a.api.LoadDomainPolicy(c.Request.Context(), id)
	}
	writeJSON(c, http.StatusCreated, agentToResponse(spec, wsID))
}

// UpdateAgent handles PATCH /api/workspaces/:id/agents/:agentId.
func (a *Adapter) UpdateAgent(c *gin.Context) {
	wsID := c.Param("id")
	tid := tenantFromGin(c)
	agentID := c.Param("agentId")

	rd, err := a.app.DomainService.Get(tid, agentID)
	if err != nil || rd == nil || rd.Spec == nil {
		errorJSON(c, http.StatusNotFound, "AGENT_NOT_FOUND", "agent not found: "+agentID)
		return
	}
	var body struct {
		Name       string `json:"name"`
		Visibility string `json:"visibility"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Name != "" {
		// Rename every agent in the harness to the new name; P0 only supports
		// the simple case where there is one canonical agent.
		for k, a := range rd.Spec.Harness.Agents {
			a.ID = body.Name
			rd.Spec.Harness.Agents[k] = a
		}
	}
	writeJSON(c, http.StatusOK, agentToResponse(rd.Spec, wsID))
}

// DeleteAgent handles DELETE /api/workspaces/:id/agents/:agentId.
func (a *Adapter) DeleteAgent(c *gin.Context) {
	tid := tenantFromGin(c)
	agentID := c.Param("agentId")
	if err := a.app.DomainService.Delete(tid, agentID); err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

// ListAgentTemplates handles GET /api/workspaces/:id/agent-templates.
//
// P0 returns an empty list; the template gallery ships in a later phase.
func (a *Adapter) ListAgentTemplates(c *gin.Context) {
	writeJSON(c, http.StatusOK, []any{})
}
