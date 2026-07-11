package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/hnsx-io/hnsx/server/internal/app/queries"
	approvalmodel "github.com/hnsx-io/hnsx/server/internal/approval/model"
	approvalrepo "github.com/hnsx-io/hnsx/server/internal/approval/repository"
	auditmodel "github.com/hnsx-io/hnsx/server/internal/audit/model"
	evalmodel "github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	evalrunner "github.com/hnsx-io/hnsx/server/internal/evaluation/runner"
	policymodel "github.com/hnsx-io/hnsx/server/internal/policy/model"
	secmodel "github.com/hnsx-io/hnsx/server/internal/secret/model"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	tracemodel "github.com/hnsx-io/hnsx/server/internal/trace/model"
	workerpkg "github.com/hnsx-io/hnsx/server/internal/worker"
)

// TemplateIndex is the shape of the template market index YAML
// (templates/index.yaml). It is duplicated here intentionally so the API
// layer can serve the gallery without importing CLI packages.
type TemplateIndex struct {
	Version   string          `yaml:"version"`
	Templates []TemplateEntry `yaml:"templates"`
}

// TemplateEntry describes one template in the index.
type TemplateEntry struct {
	ID           string               `yaml:"id"`
	Name         string               `yaml:"name"`
	Description  string               `yaml:"description"`
	Tags         []string             `yaml:"tags"`
	Source       string               `yaml:"source"`
	Variables    []TemplateVariable   `yaml:"variables"`
	Requirements TemplateRequirements `yaml:"requirements"`
}

// TemplateVariable is a user-settable placeholder in a template.
type TemplateVariable struct {
	Name    string `yaml:"name"`
	Default string `yaml:"default"`
}

// TemplateRequirements describes runtime prerequisites.
type TemplateRequirements struct {
	Providers       []string `yaml:"providers"`
	MinWorkers      int      `yaml:"min_workers"`
	SandboxRuntimes []string `yaml:"sandbox_runtimes"`
}

// ListTemplates handles GET /api/v1/templates — returns the discoverable
// template market. The response mirrors the CLI `hnsx examples` output so
// the console can render a gallery. An empty or missing index is not an
// error; it renders the "no templates" empty state.
func (s *Server) ListTemplates(c *gin.Context) {
	if s.TemplatesIndexPath == "" {
		writeJSON(c, http.StatusOK, map[string]any{
			"items": []map[string]any{},
			"total": 0,
		})
		return
	}

	idx, err := loadTemplateIndex(s.TemplatesIndexPath)
	if err != nil {
		// Missing index is OK (empty state); any other read error is 500.
		if os.IsNotExist(err) {
			writeJSON(c, http.StatusOK, map[string]any{
				"items": []map[string]any{},
				"total": 0,
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	tag := strings.TrimSpace(strings.ToLower(c.Query("tag")))
	items := make([]map[string]any, 0, len(idx.Templates))
	for _, t := range idx.Templates {
		if tag != "" && !sliceContainsFold(t.Tags, tag) {
			continue
		}
		items = append(items, map[string]any{
			"id":           t.ID,
			"name":         t.Name,
			"description":  strings.TrimSpace(t.Description),
			"tags":         t.Tags,
			"source":       t.Source,
			"variables":    templateVariablesToJSON(t.Variables),
			"requirements": templateRequirementsToJSON(t.Requirements),
		})
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

func loadTemplateIndex(path string) (*TemplateIndex, error) {
	path = filepath.Clean(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var idx TemplateIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parse template index: %w", err)
	}
	return &idx, nil
}

func templateVariablesToJSON(vars []TemplateVariable) []map[string]any {
	out := make([]map[string]any, 0, len(vars))
	for _, v := range vars {
		out = append(out, map[string]any{
			"name":    v.Name,
			"default": v.Default,
		})
	}
	return out
}

func templateRequirementsToJSON(req TemplateRequirements) map[string]any {
	return map[string]any{
		"providers":        req.Providers,
		"min_workers":      req.MinWorkers,
		"sandbox_runtimes": req.SandboxRuntimes,
	}
}

func sliceContainsFold(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.EqualFold(strings.TrimSpace(s), needle) {
			return true
		}
	}
	return false
}

// ListTraces handles GET /api/v1/traces — page of TraceSummary records
// computed directly from the observations table. The shape is the
// authoritative wire contract for the console Traces page; trace_id and
// session_id are kept distinct even though today's worker convention
// keeps them equal 1:1.
func (s *Server) ListTraces(c *gin.Context) {
	limit := intQueryGin(c, "limit", 50)
	offset := intQueryGin(c, "offset", 0)
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	from, hasFrom := parseTimeQuery(c.Query("from"))
	to, hasTo := parseTimeQuery(c.Query("to"))

	filter := tracemodel.TraceListFilter{
		TenantID:  string(tenantFromGin(c)),
		DomainID:  c.Query("domain"),
		SessionID: c.Query("session"),
		AgentID:   c.Query("agent"),
		Limit:     limit,
		Offset:    offset,
	}
	if hasFrom {
		filter.From = from
	}
	if hasTo {
		filter.To = to
	}

	if s.TraceService == nil {
		writeJSON(c, http.StatusOK, map[string]any{
			"items":  []map[string]any{},
			"total":  0,
			"limit":  limit,
			"offset": offset,
		})
		return
	}
	res, err := s.TraceService.ListSummaries(filter)
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}
	out := make([]map[string]any, 0, len(res.Summaries))
	for _, sum := range res.Summaries {
		out = append(out, traceSummaryToJSON(sum))
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items":  out,
		"total":  res.Total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetTrace handles GET /api/v1/traces/:traceId — returns the trace envelope
// with the full observation list and a per-trace rollup. 404 with the
// stable TRACE_NOT_FOUND code when the trace_id has no observations.
func (s *Server) GetTrace(c *gin.Context) {
	id := c.Param("traceId")
	if s.TraceService == nil {
		writeError(c, &APIError{
			Code:    "TRACE_NOT_FOUND",
			Message: fmt.Sprintf("trace '%s' not found", id),
		})
		return
	}
	detail, err := s.TraceService.Detail(id)
	if err != nil {
		if errors.Is(err, tracemodel.ErrTraceNotFound) {
			writeError(c, &APIError{
				Code:    "TRACE_NOT_FOUND",
				Message: fmt.Sprintf("trace '%s' not found", id),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	observations := make([]map[string]any, 0, len(detail.Observations))
	for _, rec := range detail.Observations {
		observations = append(observations, observationToMap(rec))
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"trace_id":          detail.TraceID,
		"session_id":        detail.SessionID,
		"domain_id":         detail.DomainID,
		"domain_version":    detail.DomainVersion,
		"status":            detail.Status,
		"started_at":        formatTimePtr(detail.StartedAt),
		"completed_at":      formatTimePtr(detail.CompletedAt),
		"duration_ms":       detail.DurationMs,
		"observation_count": detail.ObservationCount,
		"total_cost_usd":    detail.TotalCostUSD,
		"prompt_tokens":     detail.TotalPromptTokens,
		"completion_tokens": detail.TotalCompletionTokens,
		"agent_invocations": detail.AgentInvocations,
		"tool_invocations":  detail.ToolInvocations,
		"observations":      observations,
	})
}

func traceSummaryToJSON(sum tracemodel.TraceSummary) map[string]any {
	out := map[string]any{
		"trace_id":          sum.TraceID,
		"session_id":        sum.SessionID,
		"domain_id":         sum.DomainID,
		"domain_version":    sum.DomainVersion,
		"status":            sum.Status,
		"started_at":        formatTimePtr(sum.StartedAt),
		"completed_at":      formatTimePtr(sum.CompletedAt),
		"duration_ms":       sum.DurationMs,
		"observation_count": sum.ObservationCount,
		"total_cost_usd":    sum.TotalCostUSD,
		"prompt_tokens":     sum.TotalPromptTokens,
		"completion_tokens": sum.TotalCompletionTokens,
		"agent_invocations": sum.AgentInvocations,
		"tool_invocations":  sum.ToolInvocations,
	}
	return out
}

func formatTimePtr(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}

// ListApprovals handles GET /api/v1/approvals — the default filter is
// "pending" so the approvals inbox only ever surfaces what the operator
// still has to decide.
func (s *Server) ListApprovals(c *gin.Context) {
	if s.ApprovalService == nil {
		writeError(c, &APIError{
			Code:    "APPROVAL_UNAVAILABLE",
			Message: "approval service is not configured on this server",
		})
		return
	}
	filter := approvalrepo.ListFilter{
		DomainID:  c.Query("domain"),
		SessionID: c.Query("session"),
		Status:    c.Query("status"),
	}
	if filter.Status == "" {
		filter.Status = string(approvalmodel.StatusPending)
	}
	items, err := s.ApprovalService.List(filter)
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, approvalItemToJSON(it))
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items": out,
		"total": len(out),
	})
}

// GetApproval handles GET /api/v1/approvals/:id — returns the full
// record (including Context) so the console can render the tool payload.
func (s *Server) GetApproval(c *gin.Context) {
	if s.ApprovalService == nil {
		writeError(c, &APIError{
			Code:    "APPROVAL_UNAVAILABLE",
			Message: "approval service is not configured on this server",
		})
		return
	}
	a, err := s.ApprovalService.Get(c.Param("id"))
	if err != nil {
		if errors.Is(err, approvalmodel.ErrApprovalNotFound) {
			writeError(c, &APIError{
				Code:    "APPROVAL_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}
	writeJSON(c, http.StatusOK, approvalToJSON(a))
}

// ApproveApproval handles POST /api/v1/approvals/:id/approve.
func (s *Server) ApproveApproval(c *gin.Context) {
	if s.ApprovalService == nil {
		writeError(c, &APIError{
			Code:    "APPROVAL_UNAVAILABLE",
			Message: "approval service is not configured on this server",
		})
		return
	}
	id := c.Param("id")
	var body approvalDecisionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	reviewer := body.ReviewedBy
	if reviewer == "" {
		reviewer = "operator"
	}
	got, err := s.ApprovalService.Approve(id, reviewer, body.Comment)
	if err != nil {
		s.writeApprovalDecisionError(c, err)
		return
	}
	s.recordApprovalAudit(c, got, "approved", reviewer, body.Comment)
	writeJSON(c, http.StatusOK, approvalToJSON(got))
}

// RejectApproval handles POST /api/v1/approvals/:id/reject.
func (s *Server) RejectApproval(c *gin.Context) {
	if s.ApprovalService == nil {
		writeError(c, &APIError{
			Code:    "APPROVAL_UNAVAILABLE",
			Message: "approval service is not configured on this server",
		})
		return
	}
	id := c.Param("id")
	var body approvalDecisionBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	reviewer := body.ReviewedBy
	if reviewer == "" {
		reviewer = "operator"
	}
	got, err := s.ApprovalService.Reject(id, reviewer, body.Comment)
	if err != nil {
		s.writeApprovalDecisionError(c, err)
		return
	}
	s.recordApprovalAudit(c, got, "rejected", reviewer, body.Comment)
	writeJSON(c, http.StatusOK, approvalToJSON(got))
}

type approvalDecisionBody struct {
	ReviewedBy string `json:"reviewed_by"`
	Comment    string `json:"comment"`
}

type createApprovalBody struct {
	ID          string         `json:"id"`
	SessionID   string         `json:"session_id"`
	DomainID    string         `json:"domain_id"`
	Action      string         `json:"action"`
	Resource    string         `json:"resource"`
	RiskLevel   string         `json:"risk_level"`
	Context     map[string]any `json:"context"`
	RequestedBy string         `json:"requested_by"`
}

// CreateApproval handles POST /api/v1/approvals.
// Used by the remote worker runtime to register a human-approval gate.
func (s *Server) CreateApproval(c *gin.Context) {
	if s.ApprovalService == nil {
		writeError(c, &APIError{
			Code:    "APPROVAL_UNAVAILABLE",
			Message: "approval service is not configured on this server",
		})
		return
	}
	var body createApprovalBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	if body.ID == "" {
		body.ID = approvalmodel.NewID(body.SessionID)
	}
	risk := approvalmodel.RiskLevel(body.RiskLevel)
	if risk == "" {
		risk = approvalmodel.RiskHigh
	}
	a := &approvalmodel.Approval{
		ID:          body.ID,
		SessionID:   body.SessionID,
		DomainID:    body.DomainID,
		Action:      body.Action,
		Resource:    body.Resource,
		RiskLevel:   risk,
		Context:     body.Context,
		RequestedBy: body.RequestedBy,
	}
	if err := s.ApprovalService.Create(a); err != nil {
		writeError(c, NewInternal(err))
		return
	}
	c.Header("Location", "/api/v1/approvals/"+a.ID)
	writeJSON(c, http.StatusCreated, approvalToJSON(a))
}

// writeApprovalDecisionError centralizes the 404 / 409 mapping so the
// approve and reject handlers stay symmetric.
func (s *Server) writeApprovalDecisionError(c *gin.Context, err error) {
	if errors.Is(err, approvalmodel.ErrApprovalNotFound) {
		writeError(c, &APIError{
			Code:    "APPROVAL_NOT_FOUND",
			Message: err.Error(),
		})
		return
	}
	if errors.Is(err, approvalmodel.ErrAlreadyResolved) {
		writeError(c, &APIError{
			Code:    "APPROVAL_ALREADY_RESOLVED",
			Message: err.Error(),
		})
		return
	}
	writeError(c, NewInternal(err))
}

// recordApprovalAudit writes an immutable audit row alongside each
// approval decision so the AuditLog can attribute human-gate changes.
func (s *Server) recordApprovalAudit(c *gin.Context, a *approvalmodel.Approval, decision, reviewer, comment string) {
	if s.AuditService == nil {
		return
	}
	entry := auditmodel.Entry{
		SessionID: a.SessionID,
		DomainID:  a.DomainID,
		Action:    "approval_decision",
		Actor:     reviewer,
		ActorType: auditmodel.ActorTypeUser,
		Resource:  "approval:" + a.ID,
		Decision:  decision,
		Reason:    comment,
		Details: map[string]any{
			"approval_id": a.ID,
			"action":      a.Action,
			"resource":    a.Resource,
			"risk_level":  a.RiskLevel,
		},
	}
	_ = s.AuditService.Record(c.Request.Context(), &entry)
}

func approvalItemToJSON(it approvalmodel.ListItem) map[string]any {
	return map[string]any{
		"id":           it.ID,
		"session_id":   it.SessionID,
		"domain_id":    it.DomainID,
		"action":       it.Action,
		"resource":     it.Resource,
		"risk_level":   it.RiskLevel,
		"status":       it.Status,
		"requested_by": it.RequestedBy,
		"created_at":   it.CreatedAt,
		"updated_at":   it.UpdatedAt,
	}
}

func approvalToJSON(a *approvalmodel.Approval) map[string]any {
	if a == nil {
		return nil
	}
	out := map[string]any{
		"id":           a.ID,
		"session_id":   a.SessionID,
		"domain_id":    a.DomainID,
		"action":       a.Action,
		"resource":     a.Resource,
		"risk_level":   a.RiskLevel,
		"context":      a.Context,
		"status":       a.Status,
		"requested_by": a.RequestedBy,
		"reviewed_by":  a.ReviewedBy,
		"comment":      a.Comment,
		"created_at":   a.CreatedAt,
		"updated_at":   a.UpdatedAt,
	}
	if a.ResolvedAt != nil {
		out["resolved_at"] = *a.ResolvedAt
	}
	return out
}

// ListEvalSets handles GET /api/v1/evals.
func (s *Server) ListEvalSets(c *gin.Context) {
	limit := intQueryGin(c, "limit", 50)
	offset := intQueryGin(c, "offset", 0)
	if limit <= 0 {
		limit = 50
	}

	if s.EvalService == nil {
		writeJSON(c, http.StatusOK, map[string]any{
			"items":  []map[string]any{},
			"total":  0,
			"limit":  limit,
			"offset": offset,
		})
		return
	}

	sets, total, err := s.EvalService.ListSets(limit, offset)
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}

	out := make([]map[string]any, 0, len(sets))
	for _, set := range sets {
		out = append(out, map[string]any{
			"id":          set.ID,
			"set_id":      set.SetID,
			"domain_id":   set.DomainID,
			"description": set.Description,
			"case_count":  len(set.Cases),
			"created_at":  queries.FormatTimeValue(set.CreatedAt),
		})
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"items":  out,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// CreateEvalSet handles POST /api/v1/evals.
func (s *Server) CreateEvalSet(c *gin.Context) {
	if s.EvalService == nil {
		writeError(c, NewInternal(errors.New("eval service not configured")))
		return
	}

	var body struct {
		SetID       string               `json:"set_id"`
		DomainID    string               `json:"domain_id"`
		Description string               `json:"description,omitempty"`
		Cases       []evalmodel.EvalCase `json:"cases"`
	}
	if err := decodeJSONBody(c, &body); err != nil {
		writeError(c, NewValidation(err))
		return
	}
	if body.SetID == "" || body.DomainID == "" {
		writeError(c, NewValidation(errors.New("set_id and domain_id are required")))
		return
	}
	_, _, ok := s.Queries.GetDomain(tenantFromGin(c), body.DomainID)
	if !ok {
		writeError(c, NewDomainNotFound(body.DomainID))
		return
	}

	set := &evalmodel.EvalSet{
		ID:          body.SetID,
		SetID:       body.SetID,
		DomainID:    body.DomainID,
		Description: body.Description,
		Cases:       body.Cases,
	}
	if err := s.EvalService.CreateSet(set); err != nil {
		writeError(c, NewInternal(err))
		return
	}

	c.Header("Location", "/api/v1/evals/"+set.ID)
	writeJSON(c, http.StatusCreated, map[string]any{
		"id":         set.ID,
		"set_id":     set.SetID,
		"domain_id":  set.DomainID,
		"created_at": queries.FormatTimeValue(set.CreatedAt),
	})
}

// GetEvalSet handles GET /api/v1/evals/:setId.
func (s *Server) GetEvalSet(c *gin.Context) {
	id := c.Param("setId")
	if s.EvalService == nil {
		writeError(c, &APIError{
			Code:    "EVAL_SET_NOT_FOUND",
			Message: "eval service not configured",
		})
		return
	}

	set, err := s.EvalService.GetSet(id)
	if err != nil {
		if errors.Is(err, evalmodel.ErrEvalSetNotFound) {
			writeError(c, &APIError{
				Code:    "EVAL_SET_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	cases := make([]map[string]any, 0, len(set.Cases))
	for _, c := range set.Cases {
		cases = append(cases, map[string]any{
			"id":     c.ID,
			"name":   c.Name,
			"input":  c.Input,
			"expect": c.Expect,
			"scorer": c.Scorer,
		})
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"id":          set.ID,
		"set_id":      set.SetID,
		"domain_id":   set.DomainID,
		"description": set.Description,
		"cases":       cases,
		"created_at":  queries.FormatTimeValue(set.CreatedAt),
		"updated_at":  queries.FormatTimeValue(set.UpdatedAt),
	})
}

// UpdateEvalSet handles PUT /api/v1/evals/:setId.
func (s *Server) UpdateEvalSet(c *gin.Context) {
	id := c.Param("setId")
	if s.EvalService == nil {
		writeError(c, NewInternal(errors.New("eval service not configured")))
		return
	}

	var body struct {
		Description string               `json:"description,omitempty"`
		Cases       []evalmodel.EvalCase `json:"cases"`
	}
	if err := decodeJSONBody(c, &body); err != nil {
		writeError(c, NewValidation(err))
		return
	}

	set, err := s.EvalService.GetSet(id)
	if err != nil {
		if errors.Is(err, evalmodel.ErrEvalSetNotFound) {
			writeError(c, &APIError{
				Code:    "EVAL_SET_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	set.Description = body.Description
	set.Cases = body.Cases
	if err := s.EvalService.UpdateSet(set); err != nil {
		writeError(c, NewInternal(err))
		return
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"id":          set.ID,
		"set_id":      set.SetID,
		"domain_id":   set.DomainID,
		"description": set.Description,
		"updated_at":  queries.FormatTimeValue(set.UpdatedAt),
	})
}

// DeleteEvalSet handles DELETE /api/v1/evals/:setId.
func (s *Server) DeleteEvalSet(c *gin.Context) {
	id := c.Param("setId")
	if s.EvalService == nil {
		writeError(c, NewInternal(errors.New("eval service not configured")))
		return
	}

	if _, err := s.EvalService.GetSet(id); err != nil {
		if errors.Is(err, evalmodel.ErrEvalSetNotFound) {
			writeError(c, &APIError{
				Code:    "EVAL_SET_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	if err := s.EvalService.DeleteSet(id); err != nil {
		writeError(c, NewInternal(err))
		return
	}

	c.AbortWithStatus(http.StatusNoContent)
}

// ListEvalRuns handles GET /api/v1/evals/:setId/runs.
func (s *Server) ListEvalRuns(c *gin.Context) {
	id := c.Param("setId")
	limit := intQueryGin(c, "limit", 50)
	offset := intQueryGin(c, "offset", 0)
	if limit <= 0 {
		limit = 50
	}

	if s.EvalService == nil {
		writeJSON(c, http.StatusOK, map[string]any{
			"items":  []map[string]any{},
			"total":  0,
			"limit":  limit,
			"offset": offset,
		})
		return
	}

	if _, err := s.EvalService.GetSet(id); err != nil {
		if errors.Is(err, evalmodel.ErrEvalSetNotFound) {
			writeError(c, &APIError{
				Code:    "EVAL_SET_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	runs, err := s.EvalService.RunsBySet(id)
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}

	total := len(runs)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	runs = runs[offset:end]

	out := make([]map[string]any, 0, len(runs))
	for _, r := range runs {
		out = append(out, map[string]any{
			"id":             r.ID,
			"eval_set_id":    r.EvalSetID,
			"domain_id":      r.DomainID,
			"domain_version": r.DomainVersion,
			"orchestration":  r.Orchestration,
			"state":          r.State,
			"score":          r.Score,
			"total_cases":    r.TotalCases,
			"passed_cases":   r.PassedCases,
			"total_cost_usd": r.TotalCostUSD,
			"duration_ms":    r.DurationMs,
			"created_at":     queries.FormatTimeValue(r.CreatedAt),
			"completed_at":   queries.FormatTime(r.CompletedAt),
		})
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"items":  out,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// RunEval handles POST /api/v1/evals/:setId/run.
func (s *Server) RunEval(c *gin.Context) {
	id := c.Param("setId")
	if s.EvalService == nil {
		writeError(c, &APIError{
			Code:    "ADAPTER_NOT_IMPLEMENTED",
			Message: "eval runner not yet implemented (target: Phase 2)",
		})
		return
	}

	set, err := s.EvalService.GetSet(id)
	if err != nil {
		if errors.Is(err, evalmodel.ErrEvalSetNotFound) {
			writeError(c, &APIError{
				Code:    "EVAL_SET_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	_, domain, ok := s.Queries.GetDomain(tenantFromGin(c), set.DomainID)
	if !ok {
		writeError(c, NewDomainNotFound(set.DomainID))
		return
	}

	run := &evalmodel.EvalRun{
		ID:            uuid.NewString(),
		EvalSetID:     set.ID,
		DomainID:      set.DomainID,
		DomainVersion: domain.Version,
		Orchestration: string(domain.Spec.Harness.Session.Mode),
		State:         "running",
		TotalCases:    len(set.Cases),
	}
	if err := s.EvalService.CreateRun(run); err != nil {
		writeError(c, NewInternal(err))
		return
	}

	// The eval runner dispatches each case as a session. Prefer the worker pool
	// when available; fall back to the local executor for single-process tests.
	specForRun := domain.Spec
	budget := 0.0
	if specForRun != nil {
		budget = specForRun.Harness.Policy.Budget.MaxCostUSD
	}
	traceSvc := s.TraceService
	costFn := func(sessionID string) float64 {
		if traceSvc == nil {
			return 0
		}
		agg, err := traceSvc.Aggregate([]string{sessionID})
		if err != nil {
			return 0
		}
		return agg.TotalCostUSD
	}

	var er evalrunner.EvalRunner
	if s.WorkerService != nil && s.SessionCommands != nil {
		er = evalrunner.NewWorkerPoolRunner(s.SessionCommands, s.App.SessionService, s.EvalService, costFn)
	} else if s.Executor != nil {
		er = evalrunner.New(s.Executor, s.EvalService, evalrunner.WithCostFunc(costFn))
	} else {
		writeError(c, &APIError{
			Code:    "ADAPTER_NOT_IMPLEMENTED",
			Message: "eval runner requires a worker pool or local executor",
		})
		return
	}

	tenantID := tenantFromGin(c)
	go func() {
		ctx := tenant.NewContext(context.Background(), tenantID)
		_ = er.Run(ctx, run, set, specForRun, budget)
	}()

	c.Header("Location", fmt.Sprintf("/api/v1/evals/%s/runs/%s", set.ID, run.ID))
	writeJSON(c, http.StatusAccepted, map[string]any{
		"run_id": run.ID,
		"state":  "running",
	})
}

// GetEvalRun handles GET /api/v1/evals/:setId/runs/:runId.
func (s *Server) GetEvalRun(c *gin.Context) {
	runID := c.Param("runId")
	if s.EvalService == nil {
		writeError(c, &APIError{
			Code:    "ADAPTER_NOT_IMPLEMENTED",
			Message: "eval runner not yet implemented (target: Phase 2)",
		})
		return
	}

	run, err := s.EvalService.GetRun(runID)
	if err != nil {
		if errors.Is(err, evalmodel.ErrEvalRunNotFound) {
			writeError(c, &APIError{
				Code:    "EVAL_RUN_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	cases := make([]map[string]any, 0, len(run.Results))
	for _, res := range run.Results {
		cases = append(cases, map[string]any{
			"case_id":     res.CaseID,
			"session_id":  res.SessionID,
			"score":       res.Score,
			"passed":      res.Passed,
			"actual":      res.Actual,
			"details":     res.Details,
			"duration_ms": res.DurationMs,
			"cost_usd":    res.CostUSD,
		})
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"id":             run.ID,
		"eval_set_id":    run.EvalSetID,
		"domain_id":      run.DomainID,
		"domain_version": run.DomainVersion,
		"orchestration":  run.Orchestration,
		"state":          run.State,
		"score":          run.Score,
		"total_cases":    run.TotalCases,
		"passed_cases":   run.PassedCases,
		"total_cost_usd": run.TotalCostUSD,
		"duration_ms":    run.DurationMs,
		"cases":          cases,
		"created_at":     queries.FormatTimeValue(run.CreatedAt),
		"completed_at":   queries.FormatTime(run.CompletedAt),
	})
}

// ListAudit handles GET /api/v1/audit.
func (s *Server) ListAudit(c *gin.Context) {
	limit := intQueryGin(c, "limit", 50)
	offset := intQueryGin(c, "offset", 0)
	if limit <= 0 {
		limit = 50
	}

	if s.AuditService == nil {
		writeJSON(c, http.StatusOK, map[string]any{
			"items":  []map[string]any{},
			"total":  0,
			"limit":  limit,
			"offset": offset,
		})
		return
	}

	entries, total, err := s.AuditService.List(limit, offset)
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}

	out := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]any{
			"id":            e.ID,
			"session_id":    e.SessionID,
			"domain_id":     e.DomainID,
			"action":        e.Action,
			"actor":         e.Actor,
			"actor_type":    e.ActorType,
			"resource":      e.Resource,
			"resource_type": e.ResourceType,
			"decision":      e.Decision,
			"reason":        e.Reason,
			"details":       e.Details,
			"timestamp":     e.Timestamp.Format(time.RFC3339),
		})
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"items":  out,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetMetrics handles GET /api/v1/metrics.
func (s *Server) GetMetrics(c *gin.Context) {
	domainFilter := c.Query("domain")
	sessions := s.Queries.ListSessions(tenantFromGin(c))

	total := 0
	completed := 0
	failed := 0
	var totalDurationMs uint64
	var sessionIDs []string
	for _, sess := range sessions {
		if domainFilter != "" && sess.DomainID != domainFilter {
			continue
		}
		total++
		switch sess.State {
		case "completed":
			completed++
		case "failed":
			failed++
		}
		totalDurationMs += durationMs(sess)
		sessionIDs = append(sessionIDs, sess.ID)
	}

	var agg tracemodel.Aggregate
	if s.TraceService != nil {
		var err error
		agg, err = s.TraceService.Aggregate(sessionIDs)
		if err != nil {
			writeError(c, NewInternal(err))
			return
		}
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"domain_id":          domainFilter,
		"total_sessions":     total,
		"completed_sessions": completed,
		"failed_sessions":    failed,
		"total_cost_usd":     agg.TotalCostUSD,
		"avg_duration_ms":    avgDurationMs(totalDurationMs, total),
		"agent_invocations":  agg.AgentInvocations,
		"tool_invocations":   agg.ToolInvocations,
		"prompt_tokens":      agg.TotalPromptTokens,
		"completion_tokens":  agg.TotalCompletionTokens,
	})
}

// ListRuntimes handles GET /api/v1/runtimes — returns every live runtime
// worker that has registered with the control plane, with a freshness
// projection. WorkerService is only wired when the gRPC control plane is
// enabled (cfg.GRPCAddr != ""); in pure HTTP mode we return an empty list
// instead of a hand-rolled placeholder so the UI can render the "no
// workers yet" empty state honestly.
func (s *Server) ListRuntimes(c *gin.Context) {
	if s.WorkerService == nil {
		writeJSON(c, http.StatusOK, map[string]any{
			"items": []map[string]any{},
			"total": 0,
		})
		return
	}
	snaps := s.WorkerService.List()
	items := make([]map[string]any, 0, len(snaps))
	for _, snap := range snaps {
		items = append(items, runtimeSnapshotToJSON(snap))
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

func runtimeSnapshotToJSON(snap workerpkg.Snapshot) map[string]any {
	out := map[string]any{
		"runtime_id":        snap.WorkerID,
		"status":            runtimeStatus(snap),
		"last_heartbeat_at": snap.LastSeen.UTC().Format(time.RFC3339Nano),
		"age_seconds":       snap.AgeSeconds,
		"healthy":           snap.Healthy,
	}
	if snap.Info == nil {
		return out
	}
	if snap.Info.Version != "" {
		out["version"] = snap.Info.Version
	}
	if snap.Info.Region != "" {
		out["region"] = snap.Info.Region
	}
	if snap.Info.Hostname != "" {
		out["hostname"] = snap.Info.Hostname
	}
	if snap.Info.Pid != "" {
		out["pid"] = snap.Info.Pid
	}
	if snap.Info.Capacity != nil {
		if snap.Info.Capacity.MaxConcurrentSessions > 0 {
			out["capacity"] = snap.Info.Capacity.MaxConcurrentSessions
		}
		if len(snap.Info.Capacity.Providers) > 0 {
			out["capabilities"] = append([]string(nil), snap.Info.Capacity.Providers...)
		}
		if len(snap.Info.Capacity.Models) > 0 {
			out["models"] = append([]string(nil), snap.Info.Capacity.Models...)
		}
		if len(snap.Info.Capacity.SandboxRuntimes) > 0 {
			out["sandbox_runtimes"] = append([]string(nil), snap.Info.Capacity.SandboxRuntimes...)
		}
	}
	if len(snap.Info.Labels) > 0 {
		labels := make(map[string]string, len(snap.Info.Labels))
		for k, v := range snap.Info.Labels {
			labels[k] = v
		}
		out["labels"] = labels
	}
	return out
}

// runtimeStatus maps the registry's liveness signal into the wire-level
// status vocabulary that the UI expects:
//   - healthy       : last heartbeat within 30s
//   - degraded      : older but not yet evicted
//   - offline       : beyond the soft-eviction threshold (60s)
//
// Note: the registry's EvictStale policy decides when a worker is
// actually removed; this function only labels, it does not mutate state.
func runtimeStatus(snap workerpkg.Snapshot) string {
	if !snap.LastSeen.IsZero() && snap.AgeSeconds > 60 {
		return "offline"
	}
	if snap.Healthy {
		return "healthy"
	}
	return "degraded"
}

// ListSecrets handles GET /api/v1/secrets. The wire payload contains no
// Value/PlainValue field by design; operators verify a typed value via
// the last-4 fingerprint, never the plaintext itself.
func (s *Server) ListSecrets(c *gin.Context) {
	if s.SecretService == nil {
		writeError(c, &APIError{
			Code:    "SECRETS_UNAVAILABLE",
			Message: "secret store is not configured on this server",
		})
		return
	}
	items, err := s.SecretService.List()
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{
			"name":        it.Name,
			"description": it.Description,
			"kind":        it.Kind,
			"fingerprint": it.Fingerprint,
			"created_at":  it.CreatedAt,
			"updated_at":  it.UpdatedAt,
		})
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items": out,
		"total": len(out),
	})
}

// secretWriteBody is the request shape for Create / Update. Both calls
// accept a fresh plaintext value; the response never echoes it back.
type secretWriteBody struct {
	Name        string `json:"name"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
}

// CreateSecret handles POST /api/v1/secrets. The plaintext Value is
// required; the cipher encrypts it before it touches the repository, and
// the response is a fingerprint-only ack (no Value field).
func (s *Server) CreateSecret(c *gin.Context) {
	if s.SecretService == nil {
		writeError(c, &APIError{
			Code:    "SECRETS_UNAVAILABLE",
			Message: "secret store is not configured on this server",
		})
		return
	}
	var body secretWriteBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	if body.Name == "" || body.Value == "" {
		writeError(c, NewInvalidRequest("name and value are required"))
		return
	}
	sec := &secmodel.Secret{
		Name:        body.Name,
		Description: body.Description,
		Kind:        body.Kind,
		PlainValue:  body.Value,
	}
	if err := s.SecretService.Save(sec); err != nil {
		writeError(c, NewInternal(err))
		return
	}
	writeJSON(c, http.StatusCreated, map[string]any{
		"name":        sec.Name,
		"description": sec.Description,
		"kind":        sec.Kind,
		"fingerprint": sec.Fingerprint,
	})
}

// UpdateSecret handles PUT /api/v1/secrets/:id — replaces the value +
// metadata. Body shape is identical to Create; name in the URL is the
// authority.
func (s *Server) UpdateSecret(c *gin.Context) {
	if s.SecretService == nil {
		writeError(c, &APIError{
			Code:    "SECRETS_UNAVAILABLE",
			Message: "secret store is not configured on this server",
		})
		return
	}
	id := c.Param("id")
	var body secretWriteBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	if body.Value == "" {
		writeError(c, NewInvalidRequest("value is required for update"))
		return
	}
	sec := &secmodel.Secret{
		Name:        id,
		Description: body.Description,
		Kind:        body.Kind,
		PlainValue:  body.Value,
	}
	if err := s.SecretService.Save(sec); err != nil {
		writeError(c, NewInternal(err))
		return
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"name":        sec.Name,
		"description": sec.Description,
		"kind":        sec.Kind,
		"fingerprint": sec.Fingerprint,
	})
}

// DeleteSecret handles DELETE /api/v1/secrets/:id.
func (s *Server) DeleteSecret(c *gin.Context) {
	if s.SecretService == nil {
		writeError(c, &APIError{
			Code:    "SECRETS_UNAVAILABLE",
			Message: "secret store is not configured on this server",
		})
		return
	}
	if err := s.SecretService.Delete(c.Param("id")); err != nil {
		writeError(c, NewInternal(err))
		return
	}
	c.AbortWithStatus(http.StatusNoContent)
}

// ListPolicies handles GET /api/v1/policies.
func (s *Server) ListPolicies(c *gin.Context) {
	if s.PolicyService == nil {
		writeError(c, &APIError{
			Code:    "POLICY_UNAVAILABLE",
			Message: "policy service is not configured on this server",
		})
		return
	}
	items, err := s.PolicyService.List()
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, policyItemToJSON(it))
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items": out,
		"total": len(out),
	})
}

// CreatePolicy handles POST /api/v1/policies. Body shape:
//
//	{ "id": "no-shell",
//	  "name": "Deny Shell",
//	  "description": "...",
//	  "budget": { "max_cost_usd": 10, ... },
//	  "permissions": { ... },
//	  "guardrails": [ ... ] }
//
// id is required; the rest default to safe zero values. BoundDomain is
// intentionally NOT accepted here — bindings go through
// /domains/:id/policies so the dedicated route owns that relationship.
func (s *Server) CreatePolicy(c *gin.Context) {
	if s.PolicyService == nil {
		writeError(c, &APIError{
			Code:    "POLICY_UNAVAILABLE",
			Message: "policy service is not configured on this server",
		})
		return
	}
	var body policyWriteBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	if body.ID == "" {
		writeError(c, NewInvalidRequest("id is required"))
		return
	}
	if body.Name == "" {
		body.Name = body.ID
	}
	policy := &policymodel.Policy{
		ID:          body.ID,
		Name:        body.Name,
		Description: body.Description,
		Budget:      body.Budget,
		Permissions: body.Permissions,
		Guardrails:  body.Guardrails,
	}
	if err := s.PolicyService.CreateOrUpdate(policy); err != nil {
		writeError(c, NewInternal(err))
		return
	}
	writeJSON(c, http.StatusCreated, policyToJSON(policy))
}

// UpdatePolicy handles PUT /api/v1/policies/:id. Same body as Create.
func (s *Server) UpdatePolicy(c *gin.Context) {
	if s.PolicyService == nil {
		writeError(c, &APIError{
			Code:    "POLICY_UNAVAILABLE",
			Message: "policy service is not configured on this server",
		})
		return
	}
	id := c.Param("id")
	var body policyWriteBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	if body.ID != "" && body.ID != id {
		writeError(c, NewInvalidRequest("id in body must match id in url"))
		return
	}
	policy := &policymodel.Policy{
		ID:          id,
		Name:        body.Name,
		Description: body.Description,
		Budget:      body.Budget,
		Permissions: body.Permissions,
		Guardrails:  body.Guardrails,
	}
	if policy.Name == "" {
		policy.Name = id
	}
	if err := s.PolicyService.CreateOrUpdate(policy); err != nil {
		writeError(c, NewInternal(err))
		return
	}
	writeJSON(c, http.StatusOK, policyToJSON(policy))
}

// DeletePolicy handles DELETE /api/v1/policies/:id.
func (s *Server) DeletePolicy(c *gin.Context) {
	if s.PolicyService == nil {
		writeError(c, &APIError{
			Code:    "POLICY_UNAVAILABLE",
			Message: "policy service is not configured on this server",
		})
		return
	}
	if err := s.PolicyService.Delete(c.Param("id")); err != nil {
		if errors.Is(err, policymodel.ErrPolicyNotFound) {
			writeError(c, &APIError{
				Code:    "POLICY_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}
	c.AbortWithStatus(http.StatusNoContent)
}

// BindPolicy handles POST /api/v1/domains/:id/policies — associates
// the named policy with the domain. Body: { "policy_id": "no-shell" }.
// If the domain already had a different policy bound, that binding is
// replaced (so the 1:1 invariant holds).
func (s *Server) BindPolicy(c *gin.Context) {
	if s.PolicyService == nil {
		writeError(c, &APIError{
			Code:    "POLICY_UNAVAILABLE",
			Message: "policy service is not configured on this server",
		})
		return
	}
	domainID := c.Param("id")
	var body struct {
		PolicyID string `json:"policy_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	if body.PolicyID == "" {
		writeError(c, NewInvalidRequest("policy_id is required"))
		return
	}
	if err := s.PolicyService.BindDomain(body.PolicyID, domainID); err != nil {
		if errors.Is(err, policymodel.ErrPolicyNotFound) {
			writeError(c, &APIError{
				Code:    "POLICY_NOT_FOUND",
				Message: err.Error(),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"domain_id": domainID,
		"policy_id": body.PolicyID,
	})
}

type policyWriteBody struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Budget      policymodel.Budget      `json:"budget"`
	Permissions policymodel.Permissions `json:"permissions"`
	Guardrails  []policymodel.Guardrail `json:"guardrails"`
}

func policyItemToJSON(it policymodel.ListItem) map[string]any {
	return map[string]any{
		"id":           it.ID,
		"name":         it.Name,
		"description":  it.Description,
		"bound_domain": it.BoundDomain,
		"budget":       it.Budget,
		"permissions":  it.Permissions,
		"guardrails":   it.Guardrails,
		"created_at":   it.CreatedAt,
		"updated_at":   it.UpdatedAt,
	}
}

func policyToJSON(p *policymodel.Policy) map[string]any {
	return map[string]any{
		"id":           p.ID,
		"name":         p.Name,
		"description":  p.Description,
		"bound_domain": p.BoundDomain,
		"budget":       p.Budget,
		"permissions":  p.Permissions,
		"guardrails":   p.Guardrails,
		"created_at":   p.CreatedAt,
		"updated_at":   p.UpdatedAt,
	}
}

// ----------------------------------------------------------------------------
// tiny helpers
// ----------------------------------------------------------------------------

func intQueryGin(c *gin.Context, key string, def int) int {
	v := c.Query(key)
	if v == "" {
		return def
	}
	n := 0
	for _, ch := range v {
		if ch < '0' || ch > '9' {
			return def
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func durationMs(sess queries.SessionListItem) uint64 {
	if sess.CompletedAt == nil {
		return 0
	}
	delta := sess.CompletedAt.Sub(sess.StartedAt).Milliseconds()
	if delta < 0 {
		return 0
	}
	return uint64(delta)
}

func avgDurationMs(total uint64, n int) float64 {
	if n == 0 {
		return 0
	}
	return float64(total) / float64(n)
}

// parseTimeQuery parses an RFC3339 timestamp or a bare YYYY-MM-DD date. The
// bool reports whether a usable value was parsed.
func parseTimeQuery(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t, true
	}
	if t, err := time.Parse("2006-01-02", v); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// observationToMap renders a persisted observation record as the JSON shape used
// by the trace endpoints.
func observationToMap(rec tracemodel.ObservationRecord) map[string]any {
	return map[string]any{
		"kind":              rec.Kind,
		"trace_id":          rec.TraceID,
		"session_id":        rec.SessionID,
		"domain_id":         rec.DomainID,
		"domain_version":    rec.DomainVersion,
		"step_id":           rec.StepID,
		"agent_id":          rec.AgentID,
		"payload":           rec.Payload,
		"metadata":          rec.Metadata,
		"cost_usd":          rec.CostUSD,
		"prompt_tokens":     rec.PromptTokens,
		"completion_tokens": rec.CompletionTokens,
		"latency_ms":        rec.LatencyMs,
		"timestamp":         queries.FormatTimeValue(rec.CreatedAt),
	}
}
