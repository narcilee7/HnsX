// Package workspace defines the Workspace aggregate.
//
// A Workspace is the top-level tenancy boundary. It owns agents, issues,
// squads, daemons, and chat sessions. Workspace.Context is injected into
// every agent prompt running inside it.
//
// Persistence: the struct doubles as the GORM model. Gorm tags live here
// rather than in a parallel DTO so we don't maintain two shapes per
// entity. The tags are passive metadata — domain logic above does not
// depend on GORM at the package level.
package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrWorkspaceNotFound is returned by Repo implementations when a lookup
// misses. Defined in the domain package (not in the persistence impl) so
// transport layers can match on it without importing the infra package.
var ErrWorkspaceNotFound = errors.New("workspace: not found")

// Status enumerates the lifecycle of a workspace.
type Status string

const (
	StatusActive    Status = "active"
	StatusArchived  Status = "archived"
)

// Workspace is the aggregate root.
//
// GORM model: ID is a UUID generated app-side (google/uuid) so we don't
// need pgcrypto. Timestamps use gorm's autoCreateTime/autoUpdateTime so
// callers don't have to set them. JSONB columns (Settings) are stored as
// raw JSON.
type Workspace struct {
	ID          string         `gorm:"type:uuid;primaryKey" json:"id"`
	Name        string         `gorm:"type:text;not null" json:"name"`
	Slug        string         `gorm:"type:text;not null;uniqueIndex" json:"slug"`
	Description string         `gorm:"type:text;not null;default:''" json:"description"`
	Context     string         `gorm:"type:text;not null;default:''" json:"context"`
	Settings    json.RawMessage `gorm:"type:jsonb;not null;default:'{}'::jsonb" json:"settings"`
	Status      Status         `gorm:"type:text;not null;default:'active'" json:"status"`
	CreatedAt   time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
}

// TableName fixes the table name; otherwise GORM pluralizes to "workspaces"
// (which matches our convention anyway, but pinning it removes ambiguity).
func (Workspace) TableName() string { return "workspaces" }

// Validate enforces invariants. Called from the service layer before persistence.
func (w *Workspace) Validate() error {
	if w.Name == "" {
		return errors.New("workspace: name is required")
	}
	if w.Slug == "" {
		return errors.New("workspace: slug is required")
	}
	return nil
}

// Repo is the persistence port. Implemented by infra/db/postgres.
type Repo interface {
	Create(ctx context.Context, w *Workspace) error
	Get(ctx context.Context, id string) (*Workspace, error)
	GetBySlug(ctx context.Context, slug string) (*Workspace, error)
	List(ctx context.Context, filter ListFilter) ([]*Workspace, error)
	Update(ctx context.Context, w *Workspace) error
	Archive(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}

// ListFilter scopes a List query.
type ListFilter struct {
	Status Status
	Limit  int
	Offset int
}