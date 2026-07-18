package policy

import (
	"encoding/json"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/policy"
)

type Rule struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Expression string `json:"expression"`
	Action     string `json:"action"`
	Message    string `json:"message,omitempty"`
	Priority   int    `json:"priority"`
}

type CreateRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Rules       []Rule `json:"rules"`
}

type Response struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Rules       json.RawMessage `json:"rules"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type ListResponse struct {
	Items []Response `json:"items"`
	Total int        `json:"total"`
}

func FromDomain(p *policy.Policy) Response {
	return Response{
		ID:          p.ID,
		WorkspaceID: p.WorkspaceID,
		Name:        p.Name,
		Description: p.Description,
		Rules:       p.Rules,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func FromDomainList(items []*policy.Policy) ListResponse {
	out := ListResponse{Items: make([]Response, 0, len(items))}
	for _, p := range items {
		out.Items = append(out.Items, FromDomain(p))
	}
	out.Total = len(out.Items)
	return out
}