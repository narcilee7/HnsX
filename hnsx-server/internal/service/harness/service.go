// Package harness hosts the application-level orchestration for Harness.
package harness

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/internal/domain/harness"
)

// Service is the harness application service.
type Service struct {
	repo harness.Repo
}

// New wires a repo into a Service.
func New(repo harness.Repo) *Service { return &Service{repo: repo} }

// CreateInput mirrors the harness entity minus server-generated fields.
type CreateInput struct {
	WorkspaceID string
	Name        string
	Description string
	Prompts     []harness.Prompt
	Skills      []harness.SkillRef
	Tools       []harness.ToolRef
	PolicyID    *string
	EvalSetID   *string
	Version     string
}

// Create assigns an ID + encodes the array fields into JSONB, then persists.
func (s *Service) Create(ctx context.Context, in CreateInput) (*harness.Harness, error) {
	if in.WorkspaceID == "" {
		return nil, errors.New("harness: workspace_id is required")
	}
	h := &harness.Harness{
		ID:          uuid.NewString(),
		WorkspaceID: in.WorkspaceID,
		Name:        strings.TrimSpace(in.Name),
		Description: in.Description,
		PolicyID:    in.PolicyID,
		EvalSetID:   in.EvalSetID,
		Version:     orDefault(in.Version, "1.0.0"),
	}
	if len(in.Prompts) > 0 {
		buf, _ := json.Marshal(in.Prompts)
		h.Prompts = buf
	}
	if len(in.Skills) > 0 {
		buf, _ := json.Marshal(in.Skills)
		h.Skills = buf
	}
	if len(in.Tools) > 0 {
		buf, _ := json.Marshal(in.Tools)
		h.Tools = buf
	}
	if err := h.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, h); err != nil {
		return nil, err
	}
	return h, nil
}

// Get fetches by ID.
func (s *Service) Get(ctx context.Context, id string) (*harness.Harness, error) {
	return s.repo.Get(ctx, id)
}

// ListByWorkspace returns the harnesses in a workspace.
func (s *Service) ListByWorkspace(ctx context.Context, workspaceID string) ([]*harness.Harness, error) {
	return s.repo.ListByWorkspace(ctx, workspaceID)
}

// Update applies a patch.
func (s *Service) Update(ctx context.Context, h *harness.Harness) error {
	return s.repo.Update(ctx, h)
}

// Delete removes a harness by ID.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func orDefault(s, fb string) string {
	if s == "" {
		return fb
	}
	return s
}