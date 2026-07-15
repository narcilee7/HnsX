package multica_adapter

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/internal/approval/model"
	"github.com/hnsx-io/hnsx/server/internal/approval/repository"
	"github.com/hnsx-io/hnsx/server/internal/session/service"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// ── HarnessX Domain Registry (W13) ────────────────────────────────────────

// DomainResponse wraps an HnsX DomainSpec in the shape Multica's UI can
// render directly. It carries the full DomainSpec YAML so editors can
// show it, plus a summary of harness primitives for the gallery view.
type DomainResponse struct {
	ID            string                 `json:"id"`
	Version       string                 `json:"version"`
	Description   string                 `json:"description"`
	Spec          map[string]any         `json:"spec"`
	AgentCount    int                    `json:"agent_count"`
	SkillCount    int                    `json:"skill_count"`
	ToolCount     int                    `json:"tool_count"`
	HasPolicy     bool                   `json:"has_policy"`
	HasBudget     bool                   `json:"has_budget"`
	SessionMode   string                 `json:"session_mode"`
	WorkspaceID   string                 `json:"workspace_id"`
	CreatedAt     string                 `json:"created_at"`
	UpdatedAt     string                 `json:"updated_at"`
}

// buildDomainResponse converts an HnsX DomainSpec to Multica-friendly shape.
func buildDomainResponse(d *domain.DomainSpec, wsID string) DomainResponse {
	spec := map[string]any{
		"id":          d.ID,
		"version":     d.Version,
		"description": d.Description,
		"harness":     d.Harness,
	}
	hasBudget := d.Harness.Policy.Budget.MaxCostUSD > 0
	return DomainResponse{
		ID:          d.ID,
		Version:     d.Version,
		Description: d.Description,
		Spec:        spec,
		AgentCount:  len(d.Harness.Agents),
		SkillCount:  len(d.Harness.Skills),
		ToolCount:   len(d.Harness.Tools),
		HasPolicy:   len(d.Harness.Policy.Guardrails) > 0,
		HasBudget:   hasBudget,
		SessionMode: string(d.Harness.Session.Mode),
		WorkspaceID: wsID,
		CreatedAt:   nowISO(),
		UpdatedAt:   nowISO(),
	}
}

// ListDomains handles GET /api/harnessx/domains.
func (a *Adapter) ListDomains(c *gin.Context) {
	tid := tenantFromGin(c)
	wsID := c.Query("workspace_id")
	if wsID == "" {
		wsID = string(tid)
	}
	if a.app == nil || a.app.DomainService == nil {
		writeJSON(c, http.StatusOK, []DomainResponse{})
		return
	}
	domains, err := a.app.DomainService.List(tid)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	out := make([]DomainResponse, 0, len(domains))
	for i := range domains {
		if domains[i].Spec != nil {
			out = append(out, buildDomainResponse(domains[i].Spec, wsID))
		}
	}
	writeJSON(c, http.StatusOK, out)
}

// RegisterDomain handles POST /api/harnessx/domains.
//
// Multica body is the full DomainSpec JSON; the adapter passes it through.
func (a *Adapter) RegisterDomain(c *gin.Context) {
	tid := tenantFromGin(c)
	wsID := c.Query("workspace_id")
	if wsID == "" {
		wsID = string(tid)
	}
	var body domain.DomainSpec
	if err := c.ShouldBindJSON(&body); err != nil {
		errorJSON(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	if body.ID == "" {
		body.ID = uuid.NewString()
	}
	if body.Version == "" {
		body.Version = "0.1.0"
	}
	if a.app == nil || a.app.DomainService == nil {
		writeJSON(c, http.StatusCreated, buildDomainResponse(&body, wsID))
		return
	}
	if _, err := a.app.DomainService.Register(tid, &body); err != nil {
		errorJSON(c, http.StatusBadRequest, "VALIDATION", err.Error())
		return
	}
	if a.api != nil {
		_ = a.api.LoadDomainPolicy(c.Request.Context(), body.ID)
	}
	writeJSON(c, http.StatusCreated, buildDomainResponse(&body, wsID))
}

// GetDomain handles GET /api/harnessx/domains/:id.
func (a *Adapter) GetDomain(c *gin.Context) {
	tid := tenantFromGin(c)
	id := c.Param("id")
	if a.app == nil || a.app.DomainService == nil {
		errorJSON(c, http.StatusNotFound, "DOMAIN_NOT_FOUND", "app not wired")
		return
	}
	rd, err := a.app.DomainService.Get(tid, id)
	if err != nil || rd == nil || rd.Spec == nil {
		errorJSON(c, http.StatusNotFound, "DOMAIN_NOT_FOUND", "domain not found: "+id)
		return
	}
	writeJSON(c, http.StatusOK, buildDomainResponse(rd.Spec, string(tid)))
}

// DeleteDomain handles DELETE /api/harnessx/domains/:id.
func (a *Adapter) DeleteDomain(c *gin.Context) {
	tid := tenantFromGin(c)
	id := c.Param("id")
	if a.app == nil || a.app.DomainService == nil {
		c.Status(http.StatusNoContent)
		return
	}
	if err := a.app.DomainService.Delete(tid, id); err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

// RunDomain handles POST /api/harnessx/domains/:id/run.
func (a *Adapter) RunDomain(c *gin.Context) {
	tid := tenantFromGin(c)
	id := c.Param("id")
	var body struct {
		Trigger map[string]any `json:"trigger"`
	}
	_ = c.ShouldBindJSON(&body)
	if a.app == nil || a.app.SessionService == nil {
		writeJSON(c, http.StatusAccepted, gin.H{"queued": true, "domain_id": id})
		return
	}
	sess, err := a.app.SessionService.Create(tid, service.CreateParams{
		SessionID: uuid.NewString(),
		DomainID:  id,
		Trigger:   body.Trigger,
	})
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	writeJSON(c, http.StatusAccepted, gin.H{
		"queued":     true,
		"domain_id":  id,
		"session_id": sess.ID,
	})
}

// ── Approval Center (W6/W11) ─────────────────────────────────────────────

// ApprovalResponse is the shape Multica's UI shows in the Approvals tab.
type ApprovalResponse struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id"`
	DomainID    string `json:"domain_id"`
	Action      string `json:"action"`
	Resource    string `json:"resource"`
	RiskLevel   string `json:"risk_level"`
	Status      string `json:"status"`
	RequestedBy string `json:"requested_by"`
	CreatedAt   string `json:"created_at"`
}

// ListApprovals handles GET /api/harnessx/approvals.
func (a *Adapter) ListApprovals(c *gin.Context) {
	if a.app == nil || a.app.ApprovalService == nil {
		writeJSON(c, http.StatusOK, []ApprovalResponse{})
		return
	}
	tid := tenantFromGin(c)
	_ = tid
	apps, err := a.app.ApprovalService.List(repository.ListFilter{})
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	out := make([]ApprovalResponse, 0, len(apps))
	for _, ap := range apps {
		out = append(out, ApprovalResponse{
			ID:          ap.ID,
			SessionID:   ap.SessionID,
			DomainID:    ap.DomainID,
			Action:      ap.Action,
			Resource:    ap.Resource,
			RiskLevel:   string(ap.RiskLevel),
			Status:      string(ap.Status),
			RequestedBy: ap.RequestedBy,
			CreatedAt:   nowISO(),
		})
	}
	writeJSON(c, http.StatusOK, out)
}

// ApproveApproval handles POST /api/harnessx/approvals/:id/approve.
func (a *Adapter) ApproveApproval(c *gin.Context) {
	a.resolveApproval(c, "approved")
}

// RejectApproval handles POST /api/harnessx/approvals/:id/reject.
func (a *Adapter) RejectApproval(c *gin.Context) {
	a.resolveApproval(c, "rejected")
}

// resolveApproval is the shared approval-resolution handler.
func (a *Adapter) resolveApproval(c *gin.Context, decision string) {
	id := c.Param("id")
	if a.app == nil || a.app.ApprovalService == nil {
		writeJSON(c, http.StatusOK, gin.H{"ok": true, "decision": decision})
		return
	}
	var body struct {
		Comment string `json:"comment"`
		Actor   string `json:"actor"`
	}
	_ = c.ShouldBindJSON(&body)
	status := model.StatusApproved
	if decision == "rejected" {
		status = model.StatusRejected
	}
	if _, err := a.app.ApprovalService.Resolve(id, body.Actor, body.Comment, status); err != nil {
		errorJSON(c, http.StatusBadRequest, "RESOLVE_FAILED", err.Error())
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true, "decision": decision, "id": id})
}

// ── Cost Dashboard (W7) ──────────────────────────────────────────────────

// CostResponse is one row of the cost dashboard: per-domain spend over a
// time window.
type CostResponse struct {
	DomainID         string  `json:"domain_id"`
	TotalCostUSD     float64 `json:"total_cost_usd"`
	TotalTokens      int64   `json:"total_tokens"`
	SessionCount     int64   `json:"session_count"`
	AverageCostUSD   float64 `json:"avg_cost_usd"`
}

// CostDashboard handles GET /api/harnessx/cost/dashboard.
func (a *Adapter) CostDashboard(c *gin.Context) {
	tid := tenantFromGin(c)
	if a.app == nil || a.app.SessionService == nil {
		writeJSON(c, http.StatusOK, []CostResponse{})
		return
	}
	sessions, err := a.app.SessionService.List(tid)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	agg := map[string]*CostResponse{}
	for _, s := range sessions {
		cost := 0.0
		tokens := int64(0)
		if s.Result != nil {
			// Result doesn't track cost/tokens directly; pull from Output if
			// the session payload carried them via Session end observation.
			if v, ok := s.Result.Output["cost_usd"].(float64); ok {
				cost = v
			}
			if v, ok := s.Result.Output["total_tokens"].(float64); ok {
				tokens = int64(v)
			}
		}
		row, ok := agg[s.DomainID]
		if !ok {
			row = &CostResponse{DomainID: s.DomainID}
			agg[s.DomainID] = row
		}
		row.TotalCostUSD += cost
		row.TotalTokens += tokens
		row.SessionCount++
	}
	out := make([]CostResponse, 0, len(agg))
	for _, r := range agg {
		if r.SessionCount > 0 {
			r.AverageCostUSD = r.TotalCostUSD / float64(r.SessionCount)
		}
		out = append(out, *r)
	}
	writeJSON(c, http.StatusOK, out)
}

// ── Audit Log (W7) ──────────────────────────────────────────────────────

// AuditLog handles GET /api/harnessx/audit.
//
// Multica's UI consumes this as a paginated log of immutable events.
func (a *Adapter) AuditLog(c *gin.Context) {
	if a.app == nil || a.app.AuditService == nil {
		writeJSON(c, http.StatusOK, []any{})
		return
	}
	tid := tenantFromGin(c)
	_ = tid
	entries, _, err := a.app.AuditService.List(100, 0)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	writeJSON(c, http.StatusOK, entries)
}
