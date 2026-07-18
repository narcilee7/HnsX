// Package agent defines the Agent aggregate.
//
// An Agent is a virtual teammate that owns an HnsX Domain/Harness bundle
// (built incrementally across R2-R3) and runs against a runtime backend
// (Claude Code, Codex, Cursor, ...). Agents live inside a workspace and
// can be assigned to issues.
//
// Persistence: the struct doubles as the GORM model.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrAgentNotFound is returned by Repo implementations when a lookup misses.
var ErrAgentNotFound = errors.New("agent: not found")

// RuntimeMode picks where the agent's CLI is executed.
type RuntimeMode string

const (
	RuntimeLocal RuntimeMode = "local" // spawn CLI on the host running hnsxd daemon
	RuntimeCloud RuntimeMode = "cloud" // spawn CLI in a remote sandbox (future)
)

// Visibility controls whether an agent is shared with the whole workspace
// or kept private to its owner.
type Visibility string

const (
	VisibilityWorkspace Visibility = "workspace"
	VisibilityPrivate   Visibility = "private"
)

// Status tracks the live state of an agent runtime.
type Status string

const (
	StatusIdle     Status = "idle"
	StatusWorking  Status = "working"
	StatusBlocked  Status = "blocked"
	StatusError    Status = "error"
	StatusOffline  Status = "offline"
)

// Agent is the aggregate root.
type Agent struct {
	ID                 string          `gorm:"type:uuid;primaryKey" json:"id"`
	WorkspaceID        string          `gorm:"type:uuid;not null;index" json:"workspace_id"`
	Name               string          `gorm:"type:text;not null" json:"name"`
	Description        string          `gorm:"type:text;not null;default:''" json:"description"`
	AvatarURL          *string         `gorm:"type:text" json:"avatar_url,omitempty"`
	RuntimeMode        RuntimeMode     `gorm:"type:text;not null;default:'local'" json:"runtime_mode"`
	RuntimeConfig      json.RawMessage `gorm:"type:jsonb;not null;default:'{}'::jsonb" json:"runtime_config"`
	Visibility         Visibility      `gorm:"type:text;not null;default:'workspace'" json:"visibility"`
	Status             Status          `gorm:"type:text;not null;default:'idle';index" json:"status"`
	MaxConcurrentTasks int             `gorm:"not null;default:1" json:"max_concurrent_tasks"`
	OwnerID            *string         `gorm:"type:uuid" json:"owner_id,omitempty"`
	ArchivedAt         *time.Time      `gorm:"type:timestamptz" json:"archived_at,omitempty"`
	CreatedAt          time.Time       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt          time.Time       `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Agent) TableName() string { return "agents" }

// Validate enforces invariants.
func (a *Agent) Validate() error {
	if a.WorkspaceID == "" {
		return errors.New("agent: workspace_id is required")
	}
	if a.Name == "" {
		return errors.New("agent: name is required")
	}
	if a.MaxConcurrentTasks < 1 {
		return errors.New("agent: max_concurrent_tasks must be >= 1")
	}
	switch a.RuntimeMode {
	case RuntimeLocal, RuntimeCloud:
	default:
		return errors.New("agent: runtime_mode must be local or cloud")
	}
	switch a.Visibility {
	case VisibilityWorkspace, VisibilityPrivate:
	default:
		return errors.New("agent: visibility must be workspace or private")
	}
	return nil
}

// Repo is the persistence port.
type Repo interface {
	Create(ctx context.Context, a *Agent) error
	Get(ctx context.Context, id string) (*Agent, error)
	ListByWorkspace(ctx context.Context, workspaceID string, filter ListFilter) ([]*Agent, error)
	Update(ctx context.Context, a *Agent) error
	UpdateStatus(ctx context.Context, id string, status Status) error
	Archive(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}

// ListFilter scopes a List query.
type ListFilter struct {
	Status     Status
	Visibility Visibility
	Limit      int
	Offset     int
}