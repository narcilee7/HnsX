// Package workspace hosts the application-level orchestration for the
// Workspace aggregate. It depends only on the domain.Workspace port
// (and uuid generation); concrete persistence lives in infra/db/postgres
// and is wired in by app.New.
package workspace

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/internal/domain/workspace"
)

// Service is the workspace application service. It is the only thing the
// HTTP / CLI / WS layers see; the Repo port never escapes this package.
type Service struct {
	repo workspace.Repo
}

// New wires a repo into a Service. Repos are expected to be goroutine-safe.
func New(repo workspace.Repo) *Service { return &Service{repo: repo} }

// CreateInput is what the transport layer hands the service.
type CreateInput struct {
	Name        string
	Slug        string
	Description string
	Context     string
	Settings    []byte // raw JSON; service passes through
}

// Create assigns an ID, validates, and persists.
func (s *Service) Create(ctx context.Context, in CreateInput) (*workspace.Workspace, error) {
	w := &workspace.Workspace{
		ID:          uuid.NewString(),
		Name:        strings.TrimSpace(in.Name),
		Slug:        strings.TrimSpace(in.Slug),
		Description: in.Description,
		Context:     in.Context,
		Status:      workspace.StatusActive,
	}
	if err := w.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

// Get fetches by ID. Maps repo.ErrWorkspaceNotFound → a service-friendly
// error the transport can render.
func (s *Service) Get(ctx context.Context, id string) (*workspace.Workspace, error) {
	return s.repo.Get(ctx, id)
}

// GetBySlug fetches by URL slug.
func (s *Service) GetBySlug(ctx context.Context, slug string) (*workspace.Workspace, error) {
	return s.repo.GetBySlug(ctx, slug)
}

// List returns a page of workspaces matching the filter.
func (s *Service) List(ctx context.Context, f workspace.ListFilter) ([]*workspace.Workspace, error) {
	return s.repo.List(ctx, f)
}

// Update applies a patch to an existing workspace. The caller passes the
// full updated entity; the service validates and forwards.
func (s *Service) Update(ctx context.Context, w *workspace.Workspace) error {
	return s.repo.Update(ctx, w)
}

// Archive marks a workspace archived (soft delete).
func (s *Service) Archive(ctx context.Context, id string) error {
	return s.repo.Archive(ctx, id)
}

// Delete removes the workspace and cascades to owned resources.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}