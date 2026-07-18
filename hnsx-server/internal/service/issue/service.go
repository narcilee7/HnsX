// Package issue hosts the application-level orchestration for Issue.
package issue

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/hnsx-io/hnsx/server/internal/domain/issue"
)

// ErrAssigneeMismatch is returned by Assign when the assignee references
// an agent that does not exist (or is archived). Mapped to 400 at HTTP.
var ErrAssigneeMismatch = errors.New("issue: assignee_type=agent requires a valid agent_id")

// Service is the issue application service.
type Service struct {
	repo issue.Repo
}

// New wires a repo into a Service.
func New(repo issue.Repo) *Service { return &Service{repo: repo} }

// CreateInput is what the transport layer hands the service.
type CreateInput struct {
	WorkspaceID        string
	Title              string
	Description        string
	Status             issue.Status
	Priority           issue.Priority
	AssigneeType       *issue.AssigneeType
	AssigneeID         *string
	CreatorType        issue.CreatorType
	CreatorID          string
	ParentIssueID      *string
	AcceptanceCriteria []byte
	ContextRefs        []byte
	Position           float64
	DueDate            *string // RFC3339 string; parsed by transport
}

// Create assigns an ID + auto-number, validates, and persists.
//
// Numbering is workspace-scoped: each workspace's issues count from 1.
// We compute the next number as MAX(number)+1 within the workspace.
func (s *Service) Create(ctx context.Context, in CreateInput) (*issue.Issue, error) {
	if in.WorkspaceID == "" {
		return nil, errors.New("issue: workspace_id is required")
	}
	if in.CreatorID == "" {
		return nil, errors.New("issue: creator_id is required")
	}
	if in.Status == "" {
		in.Status = issue.StatusBacklog
	}
	if in.Priority == "" {
		in.Priority = issue.PriorityNone
	}
	if in.CreatorType == "" {
		in.CreatorType = issue.CreatorMember
	}

	number, err := s.nextNumber(ctx, in.WorkspaceID)
	if err != nil {
		return nil, err
	}

	i := &issue.Issue{
		ID:                 uuid.NewString(),
		WorkspaceID:        in.WorkspaceID,
		Number:             number,
		Title:              strings.TrimSpace(in.Title),
		Description:        in.Description,
		Status:             in.Status,
		Priority:           in.Priority,
		AssigneeType:       in.AssigneeType,
		AssigneeID:         in.AssigneeID,
		CreatorType:        in.CreatorType,
		CreatorID:          in.CreatorID,
		ParentIssueID:      in.ParentIssueID,
		Position:           in.Position,
	}
	if in.AcceptanceCriteria != nil {
		i.AcceptanceCriteria = in.AcceptanceCriteria
	}
	if in.ContextRefs != nil {
		i.ContextRefs = in.ContextRefs
	}
	if err := i.Validate(); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, i); err != nil {
		return nil, err
	}
	return i, nil
}

// nextNumber returns MAX(number)+1 for the given workspace via the repo
// port. Returns 1 when the workspace has no issues yet.
func (s *Service) nextNumber(ctx context.Context, workspaceID string) (int, error) {
	maxN, err := s.repo.MaxNumber(ctx, workspaceID)
	if err != nil {
		return 0, err
	}
	return maxN + 1, nil
}

// Get fetches by ID.
func (s *Service) Get(ctx context.Context, id string) (*issue.Issue, error) {
	return s.repo.Get(ctx, id)
}

// ListByWorkspace returns issues in a workspace matching the filter.
func (s *Service) ListByWorkspace(ctx context.Context, workspaceID string, f issue.ListFilter) ([]*issue.Issue, error) {
	return s.repo.ListByWorkspace(ctx, workspaceID, f)
}

// ListAssignedToAgent returns issues assigned to a particular agent, used
// by daemon_runtime to find work to claim.
func (s *Service) ListAssignedToAgent(ctx context.Context, agentID string, statuses []issue.Status) ([]*issue.Issue, error) {
	return s.repo.ListAssignedToAgent(ctx, agentID, statuses)
}

// Assign updates the assignee of an issue. Validates the (type, id) pair.
func (s *Service) Assign(ctx context.Context, id string, assigneeType *issue.AssigneeType, assigneeID *string) (*issue.Issue, error) {
	if assigneeType != nil && *assigneeType == issue.AssigneeAgent && (assigneeID == nil || *assigneeID == "") {
		return nil, ErrAssigneeMismatch
	}
	got, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	got.AssigneeType = assigneeType
	got.AssigneeID = assigneeID
	if got.IsAssignedToAgent() && got.Status == issue.StatusBacklog {
		// Moving from backlog to todo when assigned to an agent makes the
		// daemon eligible to claim it.
		got.Status = issue.StatusTodo
	}
	if err := s.repo.Update(ctx, got); err != nil {
		return nil, err
	}
	return got, nil
}

// Update applies a patch.
func (s *Service) Update(ctx context.Context, i *issue.Issue) error {
	return s.repo.Update(ctx, i)
}

// UpdateStatus is the canonical transition entry point.
func (s *Service) UpdateStatus(ctx context.Context, id string, status issue.Status) error {
	return s.repo.UpdateStatus(ctx, id, status)
}

// Delete removes the issue.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}