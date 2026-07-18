// Package issue defines the Issue aggregate.
//
// An Issue is a unit of work assigned to an agent or member. The lifecycle
// moves: backlog -> todo -> in_progress -> in_review -> done. Issues produce
// Observations when assigned to an agent (R3 attaches dimension tags for
// the data flywheel).
//
// Persistence: the struct doubles as the GORM model.
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
	ID                 string          `gorm:"type:uuid;primaryKey" json:"id"`
	WorkspaceID        string          `gorm:"type:uuid;not null;index" json:"workspace_id"`
	Number             int             `gorm:"not null" json:"number"`
	Title              string          `gorm:"type:text;not null" json:"title"`
	Description        string          `gorm:"type:text;not null;default:''" json:"description"`
	Status             Status          `gorm:"type:text;not null;default:'backlog';index" json:"status"`
	Priority           Priority        `gorm:"type:text;not null;default:'none'" json:"priority"`
	AssigneeType       *AssigneeType   `gorm:"type:text" json:"assignee_type,omitempty"`
	AssigneeID         *string         `gorm:"type:uuid;index" json:"assignee_id,omitempty"`
	CreatorType        CreatorType     `gorm:"type:text;not null" json:"creator_type"`
	CreatorID          string          `gorm:"type:uuid;not null" json:"creator_id"`
	ParentIssueID      *string         `gorm:"type:uuid" json:"parent_issue_id,omitempty"`
	AcceptanceCriteria json.RawMessage `gorm:"type:jsonb;not null;default:'[]'::jsonb" json:"acceptance_criteria"`
	ContextRefs        json.RawMessage `gorm:"type:jsonb;not null;default:'[]'::jsonb" json:"context_refs"`
	Position           float64         `gorm:"not null;default:0" json:"position"`
	DueDate            *time.Time      `gorm:"type:timestamptz" json:"due_date,omitempty"`
	CreatedAt          time.Time       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt          time.Time       `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Issue) TableName() string { return "issues" }

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
	// MaxNumber returns the highest issue number currently used within
	// a workspace, or 0 if no issues exist. Used by the service to assign
	// monotonic workspace-scoped numbers on Create.
	MaxNumber(ctx context.Context, workspaceID string) (int, error)
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