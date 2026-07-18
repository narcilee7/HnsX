package approval

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	adto "github.com/hnsx-io/hnsx/server/internal/api/dto/approval"
	"github.com/hnsx-io/hnsx/server/internal/domain/approval"
	asvc "github.com/hnsx-io/hnsx/server/internal/service/approval"
)

type Handler struct{ svc *asvc.Service }

func New(s *asvc.Service) *Handler { return &Handler{svc: s} }

func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/workspaces/:workspace_id/approvals")
	g.POST("", h.Request)
	g.GET("", h.ListPending)
	g.POST(":id/grant", h.Grant)
	g.POST(":id/deny", h.Deny)

	// approvals for a specific issue
	r.GET("/issues/:issue_id/approvals", h.ListByIssue)
}

func (h *Handler) Request(c *gin.Context) {
	var req adto.RequestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in := asvc.RequestInput{
		WorkspaceID: c.Param("workspace_id"),
		IssueID:     req.IssueID,
		AgentID:     req.AgentID,
		Action:      req.Action,
		Reason:      req.Reason,
		Payload:     req.Payload,
	}
	got, err := h.svc.Request(c.Request.Context(), in)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusCreated, adto.FromDomain(got))
}

func (h *Handler) ListPending(c *gin.Context) {
	items, err := h.svc.ListPending(c.Request.Context(), c.Param("workspace_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, adto.FromDomainList(items))
}

func (h *Handler) ListByIssue(c *gin.Context) {
	items, err := h.svc.ListByIssue(c.Request.Context(), c.Param("issue_id"))
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, adto.FromDomainList(items))
}

func (h *Handler) Grant(c *gin.Context) {
	var req adto.DecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	got, err := h.svc.Grant(c.Request.Context(), c.Param("id"), req.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, adto.FromDomain(got))
}

func (h *Handler) Deny(c *gin.Context) {
	var req adto.DecisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	got, err := h.svc.Deny(c.Request.Context(), c.Param("id"), req.UserID)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, adto.FromDomain(got))
}

func writeError(c *gin.Context, err error) {
	if errors.Is(err, approval.ErrApprovalNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}