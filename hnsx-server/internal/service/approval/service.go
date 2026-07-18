package approval

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/internal/domain/approval"
)

// Service is the approval application service.
type Service struct {
	repo approval.Repo
}

// New wires a repo into a Service.
func New(repo approval.Repo) *Service { return &Service{repo: repo} }

// RequestInput mirrors the approval entity minus server-generated fields.
type RequestInput struct {
	WorkspaceID string
	IssueID     string
	AgentID     string
	Action      string
	Reason      string
	Payload     []byte
	ExpiresAt   *string
}

// Request creates a pending approval. Called by the daemon when a policy
// rule with Action=approval_required fires.
func (s *Service) Request(ctx context.Context, in RequestInput) (*approval.Approval, error) {
	a := &approval.Approval{
		ID:          uuid.NewString(),
		WorkspaceID: in.WorkspaceID,
		IssueID:     in.IssueID,
		AgentID:     in.AgentID,
		Action:      strings.TrimSpace(in.Action),
		Reason:      in.Reason,
		Payload:     in.Payload,
		Status:      approval.StatusPending,
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// Grant marks an approval as granted by the given user.
func (s *Service) Grant(ctx context.Context, id, userID string) (*approval.Approval, error) {
	a, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !a.IsPending() {
		return nil, errors.New("approval: not pending")
	}
	a.Grant(userID)
	if err := s.repo.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// Deny marks an approval as denied by the given user.
func (s *Service) Deny(ctx context.Context, id, userID string) (*approval.Approval, error) {
	a, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !a.IsPending() {
		return nil, errors.New("approval: not pending")
	}
	a.Deny(userID)
	if err := s.repo.Update(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// Get fetches by ID.
func (s *Service) Get(ctx context.Context, id string) (*approval.Approval, error) {
	return s.repo.Get(ctx, id)
}

// ListByIssue returns the approvals for a specific issue.
func (s *Service) ListByIssue(ctx context.Context, issueID string) ([]*approval.Approval, error) {
	return s.repo.ListByIssue(ctx, issueID)
}

// ListPending returns the pending approvals in a workspace.
func (s *Service) ListPending(ctx context.Context, workspaceID string) ([]*approval.Approval, error) {
	return s.repo.ListPending(ctx, workspaceID)
}