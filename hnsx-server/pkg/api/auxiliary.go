package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"

	"github.com/hnsx-io/hnsx/server/internal/app/queries"
	evalmodel "github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	policymodel "github.com/hnsx-io/hnsx/server/internal/policy/model"
	"github.com/hnsx-io/hnsx/server/pkg/handler"
	viewmodel "github.com/hnsx-io/hnsx/server/pkg/handler/viewmodel"
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

	in := handler.ListTracesInput{
		TenantID:  tenantFromGin(c),
		DomainID:  c.Query("domain"),
		SessionID: c.Query("session"),
		AgentID:   c.Query("agent"),
		Limit:     limit,
		Offset:    offset,
	}
	if hasFrom {
		in.From = from
	}
	if hasTo {
		in.To = to
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
	out, err := s.Handlers.ListTraces(c.Request.Context(), in)
	if err != nil {
		if handler.IsTraceNotFound(err) {
			writeError(c, &APIError{Code: "TRACE_NOT_FOUND", Message: err.Error()})
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	items := make([]map[string]any, 0, len(out.Traces.Items))
	for _, sum := range out.Traces.Items {
		items = append(items, map[string]any{
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
			"created_at":        queries.FormatTimeValue(sum.CreatedAt),
		})
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items":  items,
		"total":  out.Traces.Total,
		"limit":  out.Traces.Limit,
		"offset": out.Traces.Offset,
	})
}

// GetTrace handles GET /api/v1/traces/:traceId — returns the trace envelope
// with the full observation list and a per-trace rollup. 404 with the
// stable TRACE_NOT_FOUND code when the trace_id has no observations.
func (s *Server) GetTrace(c *gin.Context) {
	id := c.Param("traceId")
	if s.TraceService == nil || s.Handlers == nil {
		writeError(c, &APIError{
			Code:    "TRACE_NOT_FOUND",
			Message: fmt.Sprintf("trace '%s' not found", id),
		})
		return
	}
	out, err := s.Handlers.GetTrace(c.Request.Context(), handler.GetTraceInput{
		TenantID: tenantFromGin(c),
		TraceID:  id,
	})
	if err != nil {
		if handler.IsTraceNotFound(err) {
			writeError(c, &APIError{
				Code:    "TRACE_NOT_FOUND",
				Message: fmt.Sprintf("trace '%s' not found", id),
			})
			return
		}
		writeError(c, NewInternal(err))
		return
	}

	detail := out.Trace
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
	out, err := s.Handlers.ListApprovals(c.Request.Context(), handler.ListApprovalsInput{
		TenantID:  tenantFromGin(c),
		DomainID:  c.Query("domain"),
		SessionID: c.Query("session"),
		Status:    c.Query("status"),
	})
	if err != nil {
		writeError(c, mapApprovalError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Approvals)
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
	out, err := s.Handlers.GetApproval(c.Request.Context(), handler.GetApprovalInput{
		TenantID: tenantFromGin(c),
		ID:       c.Param("id"),
	})
	if err != nil {
		writeError(c, mapApprovalError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Approval)
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
	var body struct {
		ReviewedBy string `json:"reviewed_by"`
		Comment    string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	out, err := s.Handlers.ApproveApproval(c.Request.Context(), handler.ApproveApprovalInput{
		TenantID:   tenantFromGin(c),
		ID:         c.Param("id"),
		ReviewedBy: body.ReviewedBy,
		Comment:    body.Comment,
	})
	if err != nil {
		writeError(c, mapApprovalError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Approval)
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
	var body struct {
		ReviewedBy string `json:"reviewed_by"`
		Comment    string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	out, err := s.Handlers.RejectApproval(c.Request.Context(), handler.RejectApprovalInput{
		TenantID:   tenantFromGin(c),
		ID:         c.Param("id"),
		ReviewedBy: body.ReviewedBy,
		Comment:    body.Comment,
	})
	if err != nil {
		writeError(c, mapApprovalError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Approval)
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
	var body struct {
		ID          string         `json:"id"`
		SessionID   string         `json:"session_id"`
		DomainID    string         `json:"domain_id"`
		Action      string         `json:"action"`
		Resource    string         `json:"resource"`
		RiskLevel   string         `json:"risk_level"`
		Context     map[string]any `json:"context"`
		RequestedBy string         `json:"requested_by"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	out, err := s.Handlers.CreateApproval(c.Request.Context(), handler.CreateApprovalInput{
		TenantID:    tenantFromGin(c),
		ID:          body.ID,
		SessionID:   body.SessionID,
		DomainID:    body.DomainID,
		Action:      body.Action,
		Resource:    body.Resource,
		RiskLevel:   body.RiskLevel,
		Context:     body.Context,
		RequestedBy: body.RequestedBy,
	})
	if err != nil {
		writeError(c, mapApprovalError(err))
		return
	}
	c.Header("Location", out.Location)
	writeJSON(c, http.StatusCreated, out.Approval)
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

	out, err := s.Handlers.ListEvalRuns(c.Request.Context(), handler.ListEvalRunsInput{
		TenantID: tenantFromGin(c),
		SetID:    id,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		writeError(c, mapEvalError(err))
		return
	}

	total := out.Runs.Total
	items := out.Runs.Items
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	if offset < len(items) {
		items = items[offset:end]
	} else {
		items = nil
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"items":  items,
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

	out, err := s.Handlers.RunEval(c.Request.Context(), handler.RunEvalInput{
		TenantID: tenantFromGin(c),
		SetID:    id,
	})
	if err != nil {
		writeError(c, mapEvalError(err))
		return
	}

	c.Header("Location", fmt.Sprintf("/api/v1/evals/%s/runs/%s", id, out.Run.RunID))
	writeJSON(c, http.StatusAccepted, map[string]any{
		"run_id": out.Run.RunID,
		"state":  out.Run.State,
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

	out, err := s.Handlers.GetEvalRun(c.Request.Context(), handler.GetEvalRunInput{
		TenantID: tenantFromGin(c),
		RunID:    runID,
	})
	if err != nil {
		writeError(c, mapEvalError(err))
		return
	}

	run := out.Run
	cases := make([]map[string]any, 0, len(run.Cases))
	for _, res := range run.Cases {
		cases = append(cases, map[string]any{
			"case_id":    res.CaseID,
			"session_id": res.SessionID,
			"score":      res.Score,
			"passed":     res.Passed,
			"actual":     res.Actual,
			"details":    res.Details,
		})
	}

	writeJSON(c, http.StatusOK, map[string]any{
		"id":              run.ID,
		"eval_set_id":     run.EvalSetID,
		"domain_id":       run.DomainID,
		"domain_version":  run.DomainVersion,
		"orchestration":   "",
		"state":           run.State,
		"score":           run.Score,
		"total_cases":     run.Total,
		"passed_cases":    run.Passed,
		"total_cost_usd":  run.TotalCostUSD,
		"duration_ms":     run.DurationMs,
		"baseline_run_id": run.BaselineRunID,
		"cases":           cases,
		"created_at":      queries.FormatTimeValue(time.Time{}),
		"completed_at":    "",
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

	out, err := s.Handlers.ListAudit(c.Request.Context(), handler.ListAuditInput{
		TenantID: tenantFromGin(c),
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		writeError(c, mapAuditError(err))
		return
	}

	items := make([]map[string]any, 0, len(out.Entries.Items))
	for _, e := range out.Entries.Items {
		items = append(items, map[string]any{
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
		"items":  items,
		"total":  out.Entries.Total,
		"limit":  out.Entries.Limit,
		"offset": out.Entries.Offset,
	})
}

// GetMetrics handles GET /api/v1/metrics.
func (s *Server) GetMetrics(c *gin.Context) {
	out, err := s.Handlers.GetMetrics(c.Request.Context(), handler.GetMetricsInput{
		TenantID: tenantFromGin(c),
		DomainID: c.Query("domain"),
	})
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Metrics)
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
	out, err := s.Handlers.ListRuntimes(c.Request.Context(), handler.ListRuntimesInput{
		TenantID: tenantFromGin(c),
	})
	if err != nil {
		writeError(c, NewInternal(err))
		return
	}
	items := make([]map[string]any, 0, len(out.Runtimes.Items))
	for _, it := range out.Runtimes.Items {
		items = append(items, runtimeItemToJSON(it))
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

func runtimeItemToJSON(it viewmodel.RuntimeListItem) map[string]any {
	out := map[string]any{
		"runtime_id":        it.RuntimeID,
		"status":            it.Status,
		"last_heartbeat_at": it.LastHeartbeatAt.UTC().Format(time.RFC3339Nano),
		"age_seconds":       it.AgeSeconds,
		"healthy":           it.Healthy,
	}
	if it.Version != "" {
		out["version"] = it.Version
	}
	if it.Region != "" {
		out["region"] = it.Region
	}
	if it.Hostname != "" {
		out["hostname"] = it.Hostname
	}
	if it.Pid != "" {
		out["pid"] = it.Pid
	}
	if it.Capacity > 0 {
		out["capacity"] = it.Capacity
	}
	if len(it.Capabilities) > 0 {
		out["capabilities"] = append([]string(nil), it.Capabilities...)
	}
	if len(it.Models) > 0 {
		out["models"] = append([]string(nil), it.Models...)
	}
	if len(it.SandboxRuntimes) > 0 {
		out["sandbox_runtimes"] = append([]string(nil), it.SandboxRuntimes...)
	}
	if len(it.Labels) > 0 {
		labels := make(map[string]string, len(it.Labels))
		for k, v := range it.Labels {
			labels[k] = v
		}
		out["labels"] = labels
	}
	return out
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
	out, err := s.Handlers.ListSecrets(c.Request.Context(), handler.ListSecretsInput{})
	if err != nil {
		writeError(c, mapSecretError(err))
		return
	}
	items := make([]map[string]any, 0, len(out.Secrets.Items))
	for _, it := range out.Secrets.Items {
		items = append(items, map[string]any{
			"name":        it.Name,
			"description": it.Description,
			"kind":        it.Kind,
			"fingerprint": it.Fingerprint,
			"created_at":  it.CreatedAt,
			"updated_at":  it.UpdatedAt,
		})
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
	})
}

// GetSecret handles GET /api/v1/secrets/:id.
func (s *Server) GetSecret(c *gin.Context) {
	if s.SecretService == nil {
		writeError(c, &APIError{
			Code:    "SECRETS_UNAVAILABLE",
			Message: "secret store is not configured on this server",
		})
		return
	}
	out, err := s.Handlers.GetSecret(c.Request.Context(), handler.GetSecretInput{
		Name: c.Param("id"),
	})
	if err != nil {
		writeError(c, mapSecretError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Secret)
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
	var body struct {
		Name        string `json:"name"`
		Value       string `json:"value"`
		Description string `json:"description"`
		Kind        string `json:"kind"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	out, err := s.Handlers.CreateSecret(c.Request.Context(), handler.CreateSecretInput{
		Name:        body.Name,
		Value:       body.Value,
		Description: body.Description,
		Kind:        body.Kind,
	})
	if err != nil {
		writeError(c, mapSecretError(err))
		return
	}
	writeJSON(c, http.StatusCreated, out.Secret)
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
	var body struct {
		Name        string `json:"name"`
		Value       string `json:"value"`
		Description string `json:"description"`
		Kind        string `json:"kind"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	out, err := s.Handlers.UpdateSecret(c.Request.Context(), handler.UpdateSecretInput{
		Name:        c.Param("id"),
		Value:       body.Value,
		Description: body.Description,
		Kind:        body.Kind,
	})
	if err != nil {
		writeError(c, mapSecretError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Secret)
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
	if err := s.Handlers.DeleteSecret(c.Request.Context(), handler.DeleteSecretInput{
		Name: c.Param("id"),
	}); err != nil {
		writeError(c, mapSecretError(err))
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
	out, err := s.Handlers.ListPolicies(c.Request.Context(), handler.ListPoliciesInput{})
	if err != nil {
		writeError(c, mapPolicyError(err))
		return
	}
	items := make([]map[string]any, 0, len(out.Policies.Items))
	for _, it := range out.Policies.Items {
		items = append(items, map[string]any{
			"id":           it.ID,
			"name":         it.Name,
			"description":  it.Description,
			"bound_domain": it.BoundDomain,
			"budget":       it.Budget,
			"permissions":  it.Permissions,
			"guardrails":   it.Guardrails,
			"created_at":   it.CreatedAt,
			"updated_at":   it.UpdatedAt,
		})
	}
	writeJSON(c, http.StatusOK, map[string]any{
		"items": items,
		"total": len(items),
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
	out, err := s.Handlers.CreatePolicy(c.Request.Context(), handler.CreatePolicyInput{
		ID:          body.ID,
		Name:        body.Name,
		Description: body.Description,
		Budget:      body.Budget,
		Permissions: body.Permissions,
		Guardrails:  body.Guardrails,
	})
	if err != nil {
		writeError(c, mapPolicyError(err))
		return
	}
	writeJSON(c, http.StatusCreated, out.Policy)
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
	var body policyWriteBody
	if err := c.ShouldBindJSON(&body); err != nil {
		writeError(c, NewInvalidRequest("invalid request body"))
		return
	}
	out, err := s.Handlers.UpdatePolicy(c.Request.Context(), handler.UpdatePolicyInput{
		ID:          c.Param("id"),
		BodyID:      body.ID,
		Name:        body.Name,
		Description: body.Description,
		Budget:      body.Budget,
		Permissions: body.Permissions,
		Guardrails:  body.Guardrails,
	})
	if err != nil {
		writeError(c, mapPolicyError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Policy)
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
	if err := s.Handlers.DeletePolicy(c.Request.Context(), handler.DeletePolicyInput{
		ID: c.Param("id"),
	}); err != nil {
		writeError(c, mapPolicyError(err))
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
	out, err := s.Handlers.BindPolicy(c.Request.Context(), handler.BindPolicyInput{
		DomainID: domainID,
		PolicyID: body.PolicyID,
	})
	if err != nil {
		writeError(c, mapPolicyError(err))
		return
	}
	writeJSON(c, http.StatusOK, out.Bound)
}

type policyWriteBody struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Budget      policymodel.Budget      `json:"budget"`
	Permissions policymodel.Permissions `json:"permissions"`
	Guardrails  []policymodel.Guardrail `json:"guardrails"`
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

// mapEvalError maps evaluation handler errors to stable HTTP API error codes.
func mapEvalError(err error) *APIError {
	if err == nil {
		return nil
	}
	switch {
	case handler.IsEvalSetNotFound(err):
		return &APIError{Code: "EVAL_SET_NOT_FOUND", Message: err.Error()}
	case handler.IsEvalRunNotFound(err):
		return &APIError{Code: "EVAL_RUN_NOT_FOUND", Message: err.Error()}
	case handler.IsDomainNotFound(err):
		return &APIError{Code: "DOMAIN_NOT_FOUND", Message: err.Error()}
	}
	return NewInternal(err)
}

func mapApprovalError(err error) *APIError {
	switch {
	case handler.IsApprovalNotFound(err):
		return &APIError{Code: "APPROVAL_NOT_FOUND", Message: err.Error()}
	case handler.IsApprovalAlreadyResolved(err):
		return &APIError{Code: "APPROVAL_ALREADY_RESOLVED", Message: err.Error()}
	}
	return NewInternal(err)
}

func mapSecretError(err error) *APIError {
	switch {
	case handler.IsSecretNotFound(err):
		return &APIError{Code: "SECRET_NOT_FOUND", Message: err.Error()}
	case handler.IsSecretExists(err):
		return &APIError{Code: "SECRET_EXISTS", Message: err.Error()}
	case handler.IsInvalidSecretName(err):
		return NewInvalidRequest(err.Error())
	}
	return NewInternal(err)
}

func mapPolicyError(err error) *APIError {
	switch {
	case handler.IsPolicyNotFound(err):
		return &APIError{Code: "POLICY_NOT_FOUND", Message: err.Error()}
	case handler.IsInvalidPolicyID(err):
		return NewInvalidRequest(err.Error())
	}
	return NewInternal(err)
}

func mapAuditError(err error) *APIError {
	if handler.IsAuditEntryNotFound(err) {
		return &APIError{Code: "AUDIT_ENTRY_NOT_FOUND", Message: err.Error()}
	}
	return NewInternal(err)
}

// observationToMap renders a persisted observation record as the JSON shape used
// by the trace endpoints.
func observationToMap(rec viewmodel.ObservationItem) map[string]any {
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
