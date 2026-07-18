package daemon

import (
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/daemon"
)

type RegisterRequest struct {
	Name     string `json:"name" binding:"required"`
	Platform string `json:"platform"`
	OS       string `json:"os"`
	Version  string `json:"version"`
}

type Response struct {
	ID            string    `json:"id"`
	WorkspaceID   string    `json:"workspace_id"`
	Name          string    `json:"name"`
	Platform      string    `json:"platform"`
	OS            string    `json:"os"`
	Version       string    `json:"version"`
	Status        string    `json:"status"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ListResponse struct {
	Items []Response `json:"items"`
	Total int        `json:"total"`
}

func FromDomain(d *daemon.Daemon) Response {
	return Response{
		ID:            d.ID,
		WorkspaceID:   d.WorkspaceID,
		Name:          d.Name,
		Platform:      d.Platform,
		OS:            d.OS,
		Version:       d.Version,
		Status:        string(d.Status),
		LastHeartbeat: d.LastHeartbeat,
		CreatedAt:     d.CreatedAt,
		UpdatedAt:     d.UpdatedAt,
	}
}

func FromDomainList(items []*daemon.Daemon) ListResponse {
	out := ListResponse{Items: make([]Response, 0, len(items))}
	for _, d := range items {
		out.Items = append(out.Items, FromDomain(d))
	}
	out.Total = len(out.Items)
	return out
}