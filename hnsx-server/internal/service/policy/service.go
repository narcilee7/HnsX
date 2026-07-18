package policy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/internal/domain/policy"
)

// Service is the policy application service.
type Service struct {
	repo policy.Repo
}

// New wires a repo into a Service.
func New(repo policy.Repo) *Service { return &Service{repo: repo} }

// CreateInput mirrors the policy entity minus server-generated fields.
type CreateInput struct {
	WorkspaceID string
	Name        string
	Description string
	Rules       []policy.Rule
}

// Create assigns an ID + encodes the rules array into JSONB, then persists.
func (s *Service) Create(ctx context.Context, in CreateInput) (*policy.Policy, error) {
	if in.WorkspaceID == "" {
		return nil, errors.New("policy: workspace_id is required")
	}
	p := &policy.Policy{
		ID:          uuid.NewString(),
		WorkspaceID: in.WorkspaceID,
		Name:        strings.TrimSpace(in.Name),
		Description: in.Description,
	}
	if len(in.Rules) > 0 {
		buf, _ := json.Marshal(in.Rules)
		p.Rules = buf
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Get fetches by ID.
func (s *Service) Get(ctx context.Context, id string) (*policy.Policy, error) {
	return s.repo.Get(ctx, id)
}

// ListByWorkspace returns the policies in a workspace.
func (s *Service) ListByWorkspace(ctx context.Context, workspaceID string) ([]*policy.Policy, error) {
	return s.repo.ListByWorkspace(ctx, workspaceID)
}

// Update applies a patch.
func (s *Service) Update(ctx context.Context, p *policy.Policy) error {
	return s.repo.Update(ctx, p)
}

// Delete removes a policy by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}