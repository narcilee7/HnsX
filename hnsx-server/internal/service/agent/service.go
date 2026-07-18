// Package agent hosts the application-level orchestration for Agent.
package agent

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/internal/domain/agent"
)

// Service is the agent application service.
type Service struct {
	repo agent.Repo
}

// New wires a repo into a Service.
func New(repo agent.Repo) *Service { return &Service{repo: repo} }

// CreateInput mirrors the agent entity minus server-generated fields.
type CreateInput struct {
	WorkspaceID        string
	Name               string
	Description        string
	AvatarURL          *string
	RuntimeMode        agent.RuntimeMode
	RuntimeConfig      []byte
	Visibility         agent.Visibility
	MaxConcurrentTasks int
	OwnerID            *string
}

// Create persists a new agent after validating and assigning an ID.
func (s *Service) Create(ctx context.Context, in CreateInput) (*agent.Agent, error) {
	a := &agent.Agent{
		ID:                 uuid.NewString(),
		WorkspaceID:        in.WorkspaceID,
		Name:               strings.TrimSpace(in.Name),
		Description:        in.Description,
		AvatarURL:          in.AvatarURL,
		RuntimeMode:        in.RuntimeMode,
		Visibility:         in.Visibility,
		MaxConcurrentTasks: in.MaxConcurrentTasks,
		OwnerID:            in.OwnerID,
		Status:             agent.StatusIdle,
	}
	if a.RuntimeMode == "" {
		a.RuntimeMode = agent.RuntimeLocal
	}
	if a.Visibility == "" {
		a.Visibility = agent.VisibilityWorkspace
	}
	if a.MaxConcurrentTasks == 0 {
		a.MaxConcurrentTasks = 1
	}
	if in.RuntimeConfig != nil {
		a.RuntimeConfig = in.RuntimeConfig
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// Get fetches by ID.
func (s *Service) Get(ctx context.Context, id string) (*agent.Agent, error) {
	return s.repo.Get(ctx, id)
}

// ListByWorkspace returns the agents in a workspace.
func (s *Service) ListByWorkspace(ctx context.Context, workspaceID string, f agent.ListFilter) ([]*agent.Agent, error) {
	return s.repo.ListByWorkspace(ctx, workspaceID, f)
}

// Update applies a patch to an existing agent.
func (s *Service) Update(ctx context.Context, a *agent.Agent) error {
	return s.repo.Update(ctx, a)
}

// UpdateStatus is a thin convenience wrapper used by daemon_runtime.
func (s *Service) UpdateStatus(ctx context.Context, id string, status agent.Status) error {
	return s.repo.UpdateStatus(ctx, id, status)
}

// Archive marks the agent archived + offline.
func (s *Service) Archive(ctx context.Context, id string) error {
	return s.repo.Archive(ctx, id)
}

// Delete removes the agent. Fails if referenced by issues or squads.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}