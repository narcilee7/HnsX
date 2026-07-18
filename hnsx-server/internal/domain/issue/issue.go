// Package issue defines the Issue aggregate.
//
// An Issue is a unit of work assigned to an agent or member. The lifecycle
// moves: backlog -> todo -> in_progress -> in_review -> done. Issues produce
// Observations when assigned to an agent (R3 attaches dimension tags for
// the data flywheel).
package issue

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Status enumerates the lifecycle of an issue.
type Status string

const (
	StatusBacklog    Status = "backlog"
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusInReview   Status = "in_review"
	StatusDone       Status = "done"
	StatusBlocked    Status = "blocked"
	StatusCancelled  Status = "cancelled"
)

// Priority levels.
type Priority string

const (
	PriorityUrgent Priority = "urgent"
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
	PriorityNone   Priority = "none"
)

// AssigneeType distinguishes agent-assigned from member-assigned issues.
type AssigneeType string

const (
	AssigneeMember AssigneeType = "member"
	AssigneeAgent  AssigneeType = "agent"
)

// CreatorType mirrors AssigneeType for the issue author.
type CreatorType string

const (
	CreatorMember CreatorType = "member"
	CreatorAgent  CreatorType = "agent"
)

// Issue is the aggregate root.
type Issue struct {
	ID                 string
	WorkspaceID        string
	Title              string
	Description        string
	Status             Status
	Priority           Priority
	AssigneeType       *AssigneeType
	AssigneeID         *string
	CreatorType        CreatorType
	CreatorID          string
	ParentIssueID      *string
	AcceptanceCriteria json.RawMessage
	ContextRefs        json.RawMessage
	Position           float64
	DueDate            *time.Time
	Number             int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Validate enforces invariants.
func (i *Issue) Validate() error {
	if i.WorkspaceID == "" {
		return errors.New("issue: workspace_id is required")
	}
	if i.Title == "" {
		return errors.New("issue: title is required")
	}
	switch i.Status {
	case StatusBacklog, StatusTodo, StatusInProgress, StatusInReview,
		StatusDone, StatusBlocked, StatusCancelled:
	default:
		return errors.New("issue: invalid status")
	}
	return nil
}

// IsAssigned reports whether the issue has an assignee.
func (i *Issue) IsAssigned() bool {
	return i.AssigneeType != nil && i.AssigneeID != nil
}

// IsAssignedToAgent reports whether the assignee is an agent (vs a member).
func (i *Issue) IsAssignedToAgent() bool {
	return i.AssigneeType != nil && *i.AssigneeType == AssigneeAgent && i.AssigneeID != nil
}

// Repo is the persistence port.
type Repo interface {
	Create(ctx context.Context, i *Issue) error
	Get(ctx context.Context, id string) (*Issue, error)
	ListByWorkspace(ctx context.Context, workspaceID string, filter ListFilter) ([]*Issue, error)
	ListAssignedToAgent(ctx context.Context, agentID string, statuses []Status) ([]*Issue, error)
	Update(ctx context.Context, i *Issue) error
	UpdateStatus(ctx context.Context, id string, status Status) error
	Delete(ctx context.Context, id string) error
}

// ListFilter scopes a List query.
type ListFilter struct {
	Status       Status
	AssigneeType *AssigneeType
	AssigneeID   *string
	Limit        int
	Offset       int
}