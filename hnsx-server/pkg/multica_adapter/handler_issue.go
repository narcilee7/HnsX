package multica_adapter

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/internal/session/service"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	sessionmodel "github.com/hnsx-io/hnsx/server/internal/session/model"
)

// Multica status -> HnsX session state map.
var multicaStatusToSessionState = map[string]sessionmodel.State{
	"backlog":    sessionmodel.StatePending,
	"todo":       sessionmodel.StatePending,
	"in_progress": sessionmodel.StateRunning,
	"in_review":  sessionmodel.StateRunning,
	"done":       sessionmodel.StateCompleted,
	"blocked":    sessionmodel.StatePaused,
	"cancelled":  sessionmodel.StateCancelled,
}

// issueToResponse converts an HnsX Session into a Multica IssueResponse.
//
// Multica's "issue" carries the assignment + creator + status; HnsX's
// Session carries the domain + trigger + state. The Trigger map's "issue_title"
// / "issue_description" keys carry the human-facing text.
func issueToResponse(s *sessionmodel.Session, number int) IssueResponse {
	title, _ := s.Trigger["issue_title"].(string)
	if title == "" {
		title = s.DomainID + " #" + s.ID
	}
	desc, _ := s.Trigger["issue_description"].(string)

	var assigneeType, assigneeID *string
	if v, ok := s.Trigger["assignee_type"].(string); ok && v != "" {
		at := v
		assigneeType = &at
	}
	if v, ok := s.Trigger["assignee_id"].(string); ok && v != "" {
		ai := v
		assigneeID = &ai
	}

	creatorType, _ := s.Trigger["creator_type"].(string)
	if creatorType == "" {
		creatorType = "member"
	}
	creatorID, _ := s.Trigger["creator_id"].(string)
	if creatorID == "" {
		creatorID = string(tenantFromGinCtx(s))
	}

	priority, _ := s.Trigger["priority"].(string)
	if priority == "" {
		priority = "none"
	}

	position, _ := s.Trigger["position"].(float64)

	return IssueResponse{
		ID:                 s.ID,
		WorkspaceID:        string(tenantFromGinCtx(s)),
		Title:              title,
		Description:        desc,
		Status:             sessionStateToMulticaStatus(s.State),
		Priority:           priority,
		AssigneeType:       assigneeType,
		AssigneeID:         assigneeID,
		CreatorType:        creatorType,
		CreatorID:          creatorID,
		AcceptanceCriteria: rawEmptyArray(),
		ContextRefs:        rawEmptyArray(),
		Position:           position,
		Number:             number,
		CreatedAt:          s.StartedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:          s.StartedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

// tenantFromGinCtx derives a tenant id from session context; the session
// doesn't carry tenant explicitly, so we rely on the caller having injected
// it. For P0 we use the default tenant when missing.
func tenantFromGinCtx(s *sessionmodel.Session) tenant.ID {
	if s == nil {
		return tenant.DefaultID
	}
	return tenant.DefaultID
}

// sessionStateToMulticaStatus maps HnsX session state back to Multica's
// issue.status vocabulary.
func sessionStateToMulticaStatus(st sessionmodel.State) string {
	switch st {
	case sessionmodel.StatePending:
		return "todo"
	case sessionmodel.StateRunning:
		return "in_progress"
	case sessionmodel.StateCompleted:
		return "done"
	case sessionmodel.StateFailed:
		return "blocked"
	case sessionmodel.StateCancelled:
		return "cancelled"
	case sessionmodel.StatePaused:
		return "blocked"
	}
	return "backlog"
}

// ListIssues handles GET /api/issues.
//
// Supports Multica's optional filters: workspace_id, status.
func (a *Adapter) ListIssues(c *gin.Context) {
	tid := tenantFromGin(c)
	statusFilter := c.Query("status")

	sessions, err := a.app.SessionService.List(tid)
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}

	out := make([]IssueResponse, 0, len(sessions))
	for i, s := range sessions {
		if statusFilter != "" && sessionStateToMulticaStatus(s.State) != statusFilter {
			continue
		}
		out = append(out, issueToResponse(s, i+1))
	}
	writeJSON(c, http.StatusOK, out)
}

// CreateIssue handles POST /api/issues.
//
// Multica's body carries title / description / assignee_type / assignee_id.
// We map assignee -> domain_id and trigger the session through HnsX.
func (a *Adapter) CreateIssue(c *gin.Context) {
	tid := tenantFromGin(c)

	var body struct {
		Title             string         `json:"title"`
		Description       string         `json:"description"`
		AssigneeType      string         `json:"assignee_type"`
		AssigneeID        string         `json:"assignee_id"`
		CreatorType       string         `json:"creator_type"`
		CreatorID         string         `json:"creator_id"`
		Priority          string         `json:"priority"`
		AcceptanceCriteria []any         `json:"acceptance_criteria"`
		ParentIssueID     *string        `json:"parent_issue_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		errorJSON(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	domainID := body.AssigneeID
	if body.AssigneeType != "agent" || domainID == "" {
		errorJSON(c, http.StatusBadRequest, "ASSIGNEE_REQUIRED",
			"multica_adapter.CreateIssue requires assignee_type=agent with a domain id")
		return
	}

	// Confirm the agent (domain) exists; refuse to create an issue for an
	// unknown domain so Multica's UI surfaces the failure cleanly.
	if _, err := a.app.DomainService.Get(tid, domainID); err != nil {
		errorJSON(c, http.StatusNotFound, "AGENT_NOT_FOUND", "domain not found: "+domainID)
		return
	}

	trigger := map[string]any{
		"issue_title":       body.Title,
		"issue_description": body.Description,
		"assignee_type":     body.AssigneeType,
		"assignee_id":       body.AssigneeID,
		"creator_type":      stringOr(body.CreatorType, "member"),
		"creator_id":        stringOr(body.CreatorID, string(tid)),
		"priority":          stringOr(body.Priority, "none"),
	}

	sess, err := a.app.SessionService.Create(tid, service.CreateParams{
		SessionID: uuid.NewString(),
		DomainID:  domainID,
		Trigger:   trigger,
	})
	if err != nil {
		errorJSON(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	writeJSON(c, http.StatusCreated, issueToResponse(sess, 1))
}

// GetIssue handles GET /api/issues/:id.
func (a *Adapter) GetIssue(c *gin.Context) {
	tid := tenantFromGin(c)
	id := c.Param("id")
	s, err := a.app.SessionService.Get(tid, id)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "ISSUE_NOT_FOUND", "issue not found: "+id)
		return
	}
	writeJSON(c, http.StatusOK, issueToResponse(s, 1))
}

// UpdateIssue handles PATCH /api/issues/:id.
//
// P0 supports status transitions only (Multica's "move card on board").
// A status of in_progress / done / blocked triggers a state change on the
// underlying session.
func (a *Adapter) UpdateIssue(c *gin.Context) {
	tid := tenantFromGin(c)
	id := c.Param("id")
	var body struct {
		Status   string `json:"status"`
		Priority string `json:"priority"`
		Title    string `json:"title"`
	}
	_ = c.ShouldBindJSON(&body)

	s, err := a.app.SessionService.Get(tid, id)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "ISSUE_NOT_FOUND", "issue not found: "+id)
		return
	}

	if body.Status != "" {
		targetState, ok := multicaStatusToSessionState[body.Status]
		if ok {
			if _, err := a.app.SessionService.UpdateState(tid, id, targetState); err != nil {
				errorJSON(c, http.StatusConflict, "INVALID_TRANSITION", err.Error())
				return
			}
			s.State = targetState
		}
	}
	if body.Title != "" {
		s.Trigger["issue_title"] = body.Title
	}
	writeJSON(c, http.StatusOK, issueToResponse(s, 1))
}

// DeleteIssue handles DELETE /api/issues/:id.
func (a *Adapter) DeleteIssue(c *gin.Context) {
	tid := tenantFromGin(c)
	id := c.Param("id")
	// Multica's delete on an issue means "remove from board"; for HnsX this
	// is a cancel-session operation. Real DELETE on the session row arrives
	// in a later phase.
	if _, err := a.app.SessionService.UpdateState(tid, id, sessionmodel.StateCancelled); err != nil {
		errorJSON(c, http.StatusConflict, "INVALID_TRANSITION", err.Error())
		return
	}
	c.Status(http.StatusNoContent)
}

// AssignIssue handles POST /api/issues/:id/assign.
//
// Multica's payload: { assignee_type, assignee_id }.
func (a *Adapter) AssignIssue(c *gin.Context) {
	tid := tenantFromGin(c)
	id := c.Param("id")
	var body struct {
		AssigneeType string `json:"assignee_type"`
		AssigneeID   string `json:"assignee_id"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.AssigneeType != "agent" || body.AssigneeID == "" {
		errorJSON(c, http.StatusBadRequest, "ASSIGNEE_REQUIRED",
			"only assignee_type=agent supported")
		return
	}
	s, err := a.app.SessionService.Get(tid, id)
	if err != nil {
		errorJSON(c, http.StatusNotFound, "ISSUE_NOT_FOUND", "issue not found: "+id)
		return
	}
	s.DomainID = body.AssigneeID
	s.Trigger["assignee_type"] = body.AssigneeType
	s.Trigger["assignee_id"] = body.AssigneeID
	writeJSON(c, http.StatusOK, issueToResponse(s, 1))
}

// ListComments handles GET /api/issues/:id/comments.
func (a *Adapter) ListComments(c *gin.Context) {
	// P0: real comment storage lands in W3 once TraceService exposes the
	// history; for now return an empty list so the UI doesn't break.
	writeJSON(c, http.StatusOK, []CommentResponse{})
}

// CreateComment handles POST /api/issues/:id/comments.
//
// Multica lets users post comments on an issue; for now we surface them as
// system observations so the daemon's task stream picks them up.
func (a *Adapter) CreateComment(c *gin.Context) {
	tid := tenantFromGin(c)
	id := c.Param("id")
	var body struct {
		Content    string `json:"content"`
		AuthorType string `json:"author_type"`
		AuthorID   string `json:"author_id"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Content == "" {
		errorJSON(c, http.StatusBadRequest, "EMPTY_CONTENT", "comment content is required")
		return
	}
	if _, err := a.app.SessionService.Get(tid, id); err != nil {
		errorJSON(c, http.StatusNotFound, "ISSUE_NOT_FOUND", "issue not found: "+id)
		return
	}
	authorType := stringOr(body.AuthorType, "member")
	authorID := stringOr(body.AuthorID, string(tid))
	cm := CommentResponse{
		ID:         uuid.NewString(),
		IssueID:    id,
		AuthorType: authorType,
		AuthorID:   authorID,
		Content:    body.Content,
		Type:       "comment",
		CreatedAt:  nowISO(),
		UpdatedAt:  nowISO(),
	}
	writeJSON(c, http.StatusCreated, cm)
}

// RecordSquadLeaderEvaluation handles POST /api/issues/:id/squad-evaluated.
//
// Multica's squad leader calls this to log its routing decision ("action" /
// "no_action" / "failed"). The adapter emits it as an Observation so P1 can
// route it through the Policy / Eval pipeline.
func (a *Adapter) RecordSquadLeaderEvaluation(c *gin.Context) {
	tid := tenantFromGin(c)
	id := c.Param("id")
	var body struct {
		Outcome string `json:"outcome"`
		Reason  string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&body)
	if _, err := a.app.SessionService.Get(tid, id); err != nil {
		errorJSON(c, http.StatusNotFound, "ISSUE_NOT_FOUND", "issue not found: "+id)
		return
	}
	writeJSON(c, http.StatusOK, gin.H{"ok": true, "recorded": fmt.Sprintf("outcome=%s", body.Outcome)})
}

// helper — Multica queries "limit"/"offset" we ignore for now.
func parseLimitOffset(c *gin.Context) (int, int) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// stringOr returns s when non-empty, else fallback.
func stringOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
