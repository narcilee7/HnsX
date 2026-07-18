package issue

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	issdto "github.com/hnsx-io/hnsx/server/internal/api/dto/issue"
	"github.com/hnsx-io/hnsx/server/internal/domain/issue"
	isssvc "github.com/hnsx-io/hnsx/server/internal/service/issue"
)

type Handler struct{ svc *isssvc.Service }

func New(s *isssvc.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/workspaces/:workspace_id/issues")
	g.POST("", h.Create)
	g.GET("", h.List)
	g.GET(":id", h.Get)
	g.PATCH(":id", h.Update)
	g.DELETE(":id", h.Delete)
	g.POST(":id/assign", h.Assign)

	// agent-scoped listing (daemon_runtime calls this to discover work)
	r.GET("/agents/:agent_id/issues", h.ListForAgent)
}

func (h *Handler) Create(c *gin.Context) {
	var req issdto.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in := isssvc.CreateInput{
		WorkspaceID:        c.Param("workspace_id"),
		Title:              req.Title,
		Description:        req.Description,
		Status:             issue.Status(orDefault(req.Status, string(issue.StatusBacklog))),
		Priority:           issue.Priority(orDefault(req.Priority, string(issue.PriorityNone))),
		AssigneeType:       strPtrToAssigneeType(req.AssigneeType),
		AssigneeID:         req.AssigneeID,
		CreatorType:        issue.CreatorType(orDefault(req.CreatorType, string(issue.CreatorMember))),
		CreatorID:          req.CreatorID,
		ParentIssueID:      req.ParentIssueID,
		AcceptanceCriteria: rawOrEmpty(req.AcceptanceCriteria),
		ContextRefs:        rawOrEmpty(req.ContextRefs),
		Position:           req.Position,
	}
	got, err := h.svc.Create(c.Request.Context(), in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, issdto.FromDomain(got))
}

func (h *Handler) List(c *gin.Context) {
	f := issue.ListFilter{
		Status:  issue.Status(c.Query("status")),
		Limit:   parseIntDefault(c.Query("limit"), 50),
		Offset:  parseIntDefault(c.Query("offset"), 0),
	}
	if at := c.Query("assignee_type"); at != "" {
		t := issue.AssigneeType(at)
		f.AssigneeType = &t
	}
	if aid := c.Query("assignee_id"); aid != "" {
		f.AssigneeID = &aid
	}
	items, err := h.svc.ListByWorkspace(c.Request.Context(), c.Param("workspace_id"), f)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, issdto.FromDomainList(items))
}

func (h *Handler) ListForAgent(c *gin.Context) {
	statuses := []issue.Status{issue.StatusTodo, issue.StatusInProgress}
	if ss := c.QueryArray("status"); len(ss) > 0 {
		statuses = make([]issue.Status, 0, len(ss))
		for _, s := range ss {
			statuses = append(statuses, issue.Status(s))
		}
	}
	items, err := h.svc.ListAssignedToAgent(c.Request.Context(), c.Param("agent_id"), statuses)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, issdto.FromDomainList(items))
}

func (h *Handler) Get(c *gin.Context) {
	got, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, issdto.FromDomain(got))
}

func (h *Handler) Update(c *gin.Context) {
	var req issdto.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	got, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeError(c, err)
		return
	}
	if req.Title != nil {
		got.Title = *req.Title
	}
	if req.Description != nil {
		got.Description = *req.Description
	}
	if req.Status != nil {
		got.Status = issue.Status(*req.Status)
	}
	if req.Priority != nil {
		got.Priority = issue.Priority(*req.Priority)
	}
	if req.Position != nil {
		got.Position = *req.Position
	}
	if req.AcceptanceCriteria != nil {
		got.AcceptanceCriteria = *req.AcceptanceCriteria
	}
	if req.ContextRefs != nil {
		got.ContextRefs = *req.ContextRefs
	}
	if err := h.svc.Update(c.Request.Context(), got); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, issdto.FromDomain(got))
}

func (h *Handler) Assign(c *gin.Context) {
	var req issdto.AssignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	got, err := h.svc.Assign(
		c.Request.Context(),
		c.Param("id"),
		strPtrToAssigneeType(req.AssigneeType),
		req.AssigneeID,
	)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, issdto.FromDomain(got))
}

func (h *Handler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		writeError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, issue.ErrIssueNotFound),
		errors.Is(err, issue.ErrAssigneeMismatch):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func parseIntDefault(s string, fb int) int {
	if s == "" {
		return fb
	}
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return fb
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func orDefault(s, fb string) string {
	if s == "" {
		return fb
	}
	return s
}

func strPtrToAssigneeType(s *string) *issue.AssigneeType {
	if s == nil {
		return nil
	}
	t := issue.AssigneeType(*s)
	return &t
}

func rawOrEmpty(r json.RawMessage) []byte {
	if len(r) == 0 {
		return []byte("[]")
	}
	return r
}