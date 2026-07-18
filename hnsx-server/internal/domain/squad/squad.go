// Package squad defines the Squad aggregate.
//
// A Squad is a named group of agents (and optionally members) that
// coordinate on a workflow. Squads are the unit of routing: an issue can
// be assigned to a squad and the squad's leader dispatches to members.
//
// Persistence: Squad is the GORM model; Member is serialized as JSONB
// inside the squads row for simplicity in R1.4 (no JOINs needed for the
// 90% case where we only need leader / member IDs). R3 may split into a
// separate squad_members table if querying by member becomes hot.
package squad

import (
	"context"
	"errors"
	"time"
)

// MemberRole distinguishes a leader from a member within a squad.
type MemberRole string

const (
	RoleLeader MemberRole = "leader"
	RoleMember MemberRole = "member"
)

// MemberKind distinguishes agent from member memberships.
type MemberKind string

const (
	KindAgent  MemberKind = "agent"
	KindMember MemberKind = "member"
)

// Squad is the aggregate root.
type Squad struct {
	ID          string    `gorm:"type:uuid;primaryKey" json:"id"`
	WorkspaceID string    `gorm:"type:uuid;not null;index" json:"workspace_id"`
	Name        string    `gorm:"type:text;not null" json:"name"`
	Description string    `gorm:"type:text;not null;default:''" json:"description"`
	Members     []Member  `gorm:"type:jsonb;not null;default:'[]'::jsonb;serializer:json" json:"members"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Squad) TableName() string { return "squads" }

// Member is a reference from a squad to an agent or a human member.
type Member struct {
	ID       string    `json:"id"`
	Kind     MemberKind `json:"kind"`
	Role     MemberRole `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

// Validate enforces invariants.
func (s *Squad) Validate() error {
	if s.WorkspaceID == "" {
		return errors.New("squad: workspace_id is required")
	}
	if s.Name == "" {
		return errors.New("squad: name is required")
	}
	hasLeader := false
	for _, m := range s.Members {
		if m.Role == RoleLeader {
			hasLeader = true
		}
	}
	if !hasLeader {
		return errors.New("squad: at least one leader is required")
	}
	return nil
}

// Leader returns the squad's leader (first match; squads guarantee exactly one).
func (s *Squad) Leader() (*Member, bool) {
	for i := range s.Members {
		if s.Members[i].Role == RoleLeader {
			return &s.Members[i], true
		}
	}
	return nil, false
}

// Repo is the persistence port.
type Repo interface {
	Create(ctx context.Context, s *Squad) error
	Get(ctx context.Context, id string) (*Squad, error)
	ListByWorkspace(ctx context.Context, workspaceID string) ([]*Squad, error)
	Update(ctx context.Context, s *Squad) error
	AddMember(ctx context.Context, squadID string, m Member) error
	RemoveMember(ctx context.Context, squadID, memberID string) error
	Delete(ctx context.Context, id string) error
}