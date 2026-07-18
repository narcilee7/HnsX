// Package dto holds the HTTP request/response shapes for the workspace
// resource. DTOs are deliberately separate from domain entities: the
// service / domain layers never see these structs, and these structs
// never carry DB-only fields. This keeps the API surface stable even
// when domain internals change.
package workspace

import (
	"encoding/json"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/workspace"
)

// CreateRequest is the body for POST /api/workspaces.
type CreateRequest struct {
	Name        string          `json:"name" binding:"required"`
	Slug        string          `json:"slug" binding:"required"`
	Description string          `json:"description"`
	Context     string          `json:"context"`
	Settings    json.RawMessage `json:"settings"`
}

// UpdateRequest is the body for PATCH /api/workspaces/{id}.
// Pointer fields let callers distinguish "leave unchanged" from "set to empty".
type UpdateRequest struct {
	Name        *string          `json:"name"`
	Description *string          `json:"description"`
	Context     *string          `json:"context"`
	Settings    *json.RawMessage `json:"settings"`
}

// Response is the shape returned for any single-workspace operation.
type Response struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description string          `json:"description"`
	Context     string          `json:"context"`
	Settings    json.RawMessage `json:"settings"`
	Status      string          `json:"status"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ListResponse wraps a page of workspaces.
type ListResponse struct {
	Items []Response `json:"items"`
	Total int        `json:"total"`
}

// FromDomain converts a domain entity into the response DTO.
func FromDomain(w *workspace.Workspace) Response {
	return Response{
		ID:          w.ID,
		Name:        w.Name,
		Slug:        w.Slug,
		Description: w.Description,
		Context:     w.Context,
		Settings:    w.Settings,
		Status:      string(w.Status),
		CreatedAt:   w.CreatedAt,
		UpdatedAt:   w.UpdatedAt,
	}
}

// FromDomainList converts a slice in one allocation.
func FromDomainList(ws []*workspace.Workspace) ListResponse {
	out := ListResponse{Items: make([]Response, 0, len(ws))}
	for _, w := range ws {
		out.Items = append(out.Items, FromDomain(w))
	}
	out.Total = len(out.Items)
	return out
}