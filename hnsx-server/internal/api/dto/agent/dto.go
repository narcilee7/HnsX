package agent

import (
	"encoding/json"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/agent"
)

type CreateRequest struct {
	Name               string          `json:"name" binding:"required"`
	Description        string          `json:"description"`
	AvatarURL          *string         `json:"avatar_url"`
	RuntimeMode        string          `json:"runtime_mode"`
	RuntimeConfig      json.RawMessage `json:"runtime_config"`
	Visibility         string          `json:"visibility"`
	MaxConcurrentTasks int             `json:"max_concurrent_tasks"`
	OwnerID            *string         `json:"owner_id"`
}

type UpdateRequest struct {
	Name               *string          `json:"name"`
	Description        *string          `json:"description"`
	AvatarURL          *string          `json:"avatar_url"`
	RuntimeMode        *string          `json:"runtime_mode"`
	RuntimeConfig      *json.RawMessage `json:"runtime_config"`
	Visibility         *string          `json:"visibility"`
	MaxConcurrentTasks *int             `json:"max_concurrent_tasks"`
}

type Response struct {
	ID                 string          `json:"id"`
	WorkspaceID        string          `json:"workspace_id"`
	Name               string          `json:"name"`
	Description        string          `json:"description"`
	AvatarURL          *string         `json:"avatar_url,omitempty"`
	RuntimeMode        string          `json:"runtime_mode"`
	RuntimeConfig      json.RawMessage `json:"runtime_config"`
	Visibility         string          `json:"visibility"`
	Status             string          `json:"status"`
	MaxConcurrentTasks int             `json:"max_concurrent_tasks"`
	OwnerID            *string         `json:"owner_id,omitempty"`
	ArchivedAt         *time.Time      `json:"archived_at,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

type ListResponse struct {
	Items []Response `json:"items"`
	Total int        `json:"total"`
}

func FromDomain(a *agent.Agent) Response {
	return Response{
		ID:                 a.ID,
		WorkspaceID:        a.WorkspaceID,
		Name:               a.Name,
		Description:        a.Description,
		AvatarURL:          a.AvatarURL,
		RuntimeMode:        string(a.RuntimeMode),
		RuntimeConfig:      a.RuntimeConfig,
		Visibility:         string(a.Visibility),
		Status:             string(a.Status),
		MaxConcurrentTasks: a.MaxConcurrentTasks,
		OwnerID:            a.OwnerID,
		ArchivedAt:         a.ArchivedAt,
		CreatedAt:          a.CreatedAt,
		UpdatedAt:          a.UpdatedAt,
	}
}

func FromDomainList(items []*agent.Agent) ListResponse {
	out := ListResponse{Items: make([]Response, 0, len(items))}
	for _, a := range items {
		out.Items = append(out.Items, FromDomain(a))
	}
	out.Total = len(out.Items)
	return out
}