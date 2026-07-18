package squad

import (
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/squad"
)

type Member struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type CreateRequest struct {
	Name        string  `json:"name" binding:"required"`
	Description string  `json:"description"`
	Members     []Member `json:"members"`
}

type UpdateRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Members     []Member `json:"members"`
}

type AddMemberRequest struct {
	Member Member `json:"member"`
}

type Response struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Members     []Member  `json:"members"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ListResponse struct {
	Items []Response `json:"items"`
	Total int        `json:"total"`
}

func FromDomain(s *squad.Squad) Response {
	out := Response{
		ID:          s.ID,
		WorkspaceID: s.WorkspaceID,
		Name:        s.Name,
		Description: s.Description,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.UpdatedAt,
	}
	out.Members = make([]Member, 0, len(s.Members))
	for _, m := range s.Members {
		out.Members = append(out.Members, Member{
			ID: m.ID, Kind: string(m.Kind), Role: string(m.Role), JoinedAt: m.JoinedAt,
		})
	}
	return out
}

func FromDomainList(items []*squad.Squad) ListResponse {
	out := ListResponse{Items: make([]Response, 0, len(items))}
	for _, s := range items {
		out.Items = append(out.Items, FromDomain(s))
	}
	out.Total = len(out.Items)
	return out
}

func (m Member) ToDomain() squad.Member {
	return squad.Member{
		ID: m.ID,
		Kind: squad.MemberKind(m.Kind),
		Role: squad.MemberRole(m.Role),
		JoinedAt: m.JoinedAt,
	}
}