package harness

import (
	"encoding/json"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/harness"
)

type Prompt struct {
	Name     string   `json:"name"`
	Template string   `json:"template"`
	Vars     []string `json:"vars,omitempty"`
}

type SkillRef struct {
	SkillID string `json:"skill_id"`
	Version string `json:"version"`
}

type ToolRef struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
}

type CreateRequest struct {
	Name        string     `json:"name" binding:"required"`
	Description string     `json:"description"`
	Prompts     []Prompt   `json:"prompts"`
	Skills      []SkillRef `json:"skills"`
	Tools       []ToolRef  `json:"tools"`
	PolicyID    *string    `json:"policy_id"`
	EvalSetID   *string    `json:"eval_set_id"`
	Version     string     `json:"version"`
}

type UpdateRequest struct {
	Name        *string     `json:"name"`
	Description *string     `json:"description"`
	Prompts     []Prompt    `json:"prompts"`
	Skills      []SkillRef  `json:"skills"`
	Tools       []ToolRef   `json:"tools"`
	PolicyID    *string     `json:"policy_id"`
	EvalSetID   *string     `json:"eval_set_id"`
}

type Response struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Prompts     json.RawMessage `json:"prompts"`
	Skills      json.RawMessage `json:"skills"`
	Tools       json.RawMessage `json:"tools"`
	PolicyID    *string         `json:"policy_id,omitempty"`
	EvalSetID   *string         `json:"eval_set_id,omitempty"`
	Version     string          `json:"version"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type ListResponse struct {
	Items []Response `json:"items"`
	Total int        `json:"total"`
}

func FromDomain(h *harness.Harness) Response {
	return Response{
		ID:          h.ID,
		WorkspaceID: h.WorkspaceID,
		Name:        h.Name,
		Description: h.Description,
		Prompts:     h.Prompts,
		Skills:      h.Skills,
		Tools:       h.Tools,
		PolicyID:    h.PolicyID,
		EvalSetID:   h.EvalSetID,
		Version:     h.Version,
		CreatedAt:   h.CreatedAt,
		UpdatedAt:   h.UpdatedAt,
	}
}

func FromDomainList(items []*harness.Harness) ListResponse {
	out := ListResponse{Items: make([]Response, 0, len(items))}
	for _, h := range items {
		out.Items = append(out.Items, FromDomain(h))
	}
	out.Total = len(out.Items)
	return out
}