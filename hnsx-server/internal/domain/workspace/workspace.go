// Package workspace defines the Workspace aggregate.
//
// A Workspace is the top-level tenancy boundary. It owns agents, issues,
// squads, daemons, and chat sessions. Workspace.Context is injected into
// every agent prompt running inside it.
package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Status enumerates the lifecycle of a workspace.
type Status string

const (
	StatusActive    Status = "active"
	StatusArchived  Status = "archived"
)

// Workspace is the aggregate root.
type Workspace struct {
	ID          string
	Name        string
	Slug        string
	Description string
	Context     string          // workspace-level system prompt injected into agent runs
	Settings    json.RawMessage // free-form settings (theme, defaults, integrations)
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

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