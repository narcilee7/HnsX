package approval

import (
	"encoding/json"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/approval"
)

type RequestRequest struct {
	IssueID string          `json:"issue_id" binding:"required"`
	AgentID string          `json:"agent_id" binding:"required"`
	Action  string          `json:"action" binding:"required"`
	Reason  string          `json:"reason"`
	Payload json.RawMessage `json:"payload"`
}

type DecisionRequest struct {
	UserID string `json:"user_id" binding:"required"`
}

type Response struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	IssueID     string          `json:"issue_id"`
	AgentID     string          `json:"agent_id"`
	Action      string          `json:"action"`
	Reason      string          `json:"reason"`
	Payload     json.RawMessage `json:"payload"`
	Status      string          `json:"status"`
	RequestedAt time.Time       `json:"requested_at"`
	DecidedAt   *time.Time      `json:"decided_at,omitempty"`
	DecidedBy   *string         `json:"decided_by,omitempty"`
	ExpiresAt   *time.Time      `json:"expires_at,omitempty"`
}

type ListResponse struct {
	Items []Response `json:"items"`
	Total int        `json:"total"`
}

func FromDomain(a *approval.Approval) Response {
	return Response{
		ID:          a.ID,
		WorkspaceID: a.WorkspaceID,
		IssueID:     a.IssueID,
		AgentID:     a.AgentID,
		Action:      a.Action,
		Reason:      a.Reason,
		Payload:     a.Payload,
		Status:      string(a.Status),
		RequestedAt: a.RequestedAt,
		DecidedAt:   a.DecidedAt,
		DecidedBy:   a.DecidedBy,
		ExpiresAt:   a.ExpiresAt,
	}
}

func FromDomainList(items []*approval.Approval) ListResponse {
	out := ListResponse{Items: make([]Response, 0, len(items))}
	for _, a := range items {
		out.Items = append(out.Items, FromDomain(a))
	}
	out.Total = len(out.Items)
	return out
}