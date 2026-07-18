// Package squad hosts the application-level orchestration for Squad.
package squad

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/internal/domain/squad"
)

// Service is the squad application service.
type Service struct {
	repo squad.Repo
}

// New wires a repo into a Service.
func New(repo squad.Repo) *Service { return &Service{repo: repo} }

// CreateInput mirrors the squad entity minus server-generated fields.
type CreateInput struct {
	WorkspaceID string
	Name        string
	Description string
	Members     []squad.Member
}

// Create persists a new squad with a generated ID.
func (s *Service) Create(ctx context.Context, in CreateInput) (*squad.Squad, error) {
	if in.WorkspaceID == "" {
		return nil, errors.New("squad: workspace_id is required")
	}
	for i := range in.Members {
		if in.Members[i].JoinedAt.IsZero() {
			in.Members[i].JoinedAt = time.Now().UTC()
		}
	}
	sq := &squad.Squad{
		ID:          uuid.NewString(),
		WorkspaceID: in.WorkspaceID,
		Name:        strings.TrimSpace(in.Name),
		Description: in.Description,
		Members:     in.Members,
	}
	if err := sq.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, sq); err != nil {
		return nil, err
	}
	return sq, nil
}

// Get fetches by ID.
func (s *Service) Get(ctx context.Context, id string) (*squad.Squad, error) {
	return s.repo.Get(ctx, id)
}

// ListByWorkspace returns all squads in a workspace.
func (s *Service) ListByWorkspace(ctx context.Context, workspaceID string) ([]*squad.Squad, error) {
	return s.repo.ListByWorkspace(ctx, workspaceID)
}

// Update applies a patch.
func (s *Service) Update(ctx context.Context, sq *squad.Squad) error {
	return s.repo.Update(ctx, sq)
}

// AddMember appends a member to the squad. Idempotent on duplicate IDs.
func (s *Service) AddMember(ctx context.Context, squadID string, m squad.Member) error {
	return s.repo.AddMember(ctx, squadID, m)
}

// RemoveMember drops a member by ID. No-op if not present.
func (s *Service) RemoveMember(ctx context.Context, squadID, memberID string) error {
	return s.repo.RemoveMember(ctx, squadID, memberID)
}

// Delete removes the squad.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}