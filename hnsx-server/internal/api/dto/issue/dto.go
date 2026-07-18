package issue

import (
	"encoding/json"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/issue"
)

type CreateRequest struct {
	Title              string          `json:"title" binding:"required"`
	Description        string          `json:"description"`
	Status             string          `json:"status"`
	Priority           string          `json:"priority"`
	AssigneeType       *string         `json:"assignee_type"`
	AssigneeID         *string         `json:"assignee_id"`
	CreatorType        string          `json:"creator_type"`
	CreatorID          string          `json:"creator_id" binding:"required"`
	ParentIssueID      *string         `json:"parent_issue_id"`
	AcceptanceCriteria json.RawMessage `json:"acceptance_criteria"`
	ContextRefs        json.RawMessage `json:"context_refs"`
	Position           float64         `json:"position"`
}

type UpdateRequest struct {
	Title              *string          `json:"title"`
	Description        *string          `json:"description"`
	Status             *string          `json:"status"`
	Priority           *string          `json:"priority"`
	AcceptanceCriteria *json.RawMessage `json:"acceptance_criteria"`
	ContextRefs        *json.RawMessage `json:"context_refs"`
	Position           *float64         `json:"position"`
}

type AssignRequest struct {
	AssigneeType *string `json:"assignee_type"`
	AssigneeID   *string `json:"assignee_id"`
}

type Response struct {
	ID                 string          `json:"id"`
	WorkspaceID        string          `json:"workspace_id"`
	Number             int             `json:"number"`
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	Status             string          `json:"status"`
	Priority           string          `json:"priority"`
	AssigneeType       *string         `json:"assignee_type,omitempty"`
	AssigneeID         *string         `json:"assignee_id,omitempty"`
	CreatorType        string          `json:"creator_type"`
	CreatorID          string          `json:"creator_id"`
	ParentIssueID      *string         `json:"parent_issue_id,omitempty"`
	AcceptanceCriteria json.RawMessage `json:"acceptance_criteria"`
	ContextRefs        json.RawMessage `json:"context_refs"`
	Position           float64         `json:"position"`
	DueDate            *time.Time      `json:"due_date,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type ListResponse struct {
	Items []Response `json:"items"`
	Total int        `json:"total"`
}

func FromDomain(i *issue.Issue) Response {
	return Response{
		ID:                 i.ID,
		WorkspaceID:        i.WorkspaceID,
		Number:             i.Number,
		Title:              i.Title,
		Description:        i.Description,
		Status:             string(i.Status),
		Priority:           string(i.Priority),
		AssigneeType:       typeStringPtr(i.AssigneeType),
		AssigneeID:         i.AssigneeID,
		CreatorType:        string(i.CreatorType),
		CreatorID:          i.CreatorID,
		ParentIssueID:      i.ParentIssueID,
		AcceptanceCriteria: i.AcceptanceCriteria,
		ContextRefs:        i.ContextRefs,
		Position:           i.Position,
		DueDate:            i.DueDate,
		CreatedAt:          i.CreatedAt,
		UpdatedAt:          i.UpdatedAt,
	}
}

func FromDomainList(items []*issue.Issue) ListResponse {
	out := ListResponse{Items: make([]Response, 0, len(items))}
	for _, i := range items {
		out.Items = append(out.Items, FromDomain(i))
	}
	out.Total = len(out.Items)
	return out
}

func typeStringPtr(t *issue.AssigneeType) *string {
	if t == nil {
		return nil
	}
	s := string(*t)
	return &s
}