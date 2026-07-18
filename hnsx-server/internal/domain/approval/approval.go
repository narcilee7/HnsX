// Package approval defines the Approval aggregate — the human-in-the-loop
// gate for actions the policy engine flags as approval_required.
//
// When a tool call exceeds a cost ceiling or matches a guardrail rule
// with Action=approval_required, the daemon_runtime pauses the agent,
// records a KindApprovalEvent Observation with status=pending, and
// waits on an external approval (webhook, console button, ...).
//
// R3 lands the data model + port. The wait/notify mechanism (webhook
// handlers, Slack/Feishu integration) lands in R3.x as a thin transport
// on top.
package approval

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// ErrApprovalNotFound is returned by Repo implementations when a lookup misses.
var ErrApprovalNotFound = errors.New("approval: not found")

// Status tracks the lifecycle of an Approval request.
type Status string

const (
	StatusPending  Status = "pending"
	StatusGranted  Status = "granted"
	StatusDenied   Status = "denied"
	StatusExpired  Status = "expired"
)

// Approval is the aggregate root.
type Approval struct {
	ID          string          `gorm:"type:uuid;primaryKey" json:"id"`
	WorkspaceID string          `gorm:"type:uuid;not null;index" json:"workspace_id"`
	IssueID     string          `gorm:"type:uuid;not null;index" json:"issue_id"`
	AgentID     string          `gorm:"type:uuid;not null" json:"agent_id"`
	Action      string          `gorm:"type:text;not null" json:"action"`     // e.g. "tool_call:Write"
	Reason      string          `gorm:"type:text;not null" json:"reason"`
	Payload     json.RawMessage `gorm:"type:jsonb;not null;default:'{}'::jsonb" json:"payload"`
	Status      Status          `gorm:"type:text;not null;default:'pending';index" json:"status"`
	RequestedAt time.Time       `gorm:"autoCreateTime" json:"requested_at"`
	DecidedAt   *time.Time      `gorm:"type:timestamptz" json:"decided_at,omitempty"`
	DecidedBy   *string         `gorm:"type:uuid" json:"decided_by,omitempty"`
	ExpiresAt   *time.Time      `gorm:"type:timestamptz" json:"expires_at,omitempty"`
}

func (Approval) TableName() string { return "approvals" }

// Validate enforces invariants.
func (a *Approval) Validate() error {
	if a.WorkspaceID == "" {
		return errors.New("approval: workspace_id is required")
	}
	if a.IssueID == "" {
		return errors.New("approval: issue_id is required")
	}
	if a.AgentID == "" {
		return errors.New("approval: agent_id is required")
	}
	if a.Action == "" {
		return errors.New("approval: action is required")
	}
	return nil
}

// IsPending reports whether the approval is still waiting.
func (a *Approval) IsPending() bool { return a.Status == StatusPending }

// Grant marks the approval as granted by the given user.
func (a *Approval) Grant(userID string) {
	now := time.Now().UTC()
	a.Status = StatusGranted
	a.DecidedAt = &now
	a.DecidedBy = &userID
}

// Deny marks the approval as denied by the given user.
func (a *Approval) Deny(userID string) {
	now := time.Now().UTC()
	a.Status = StatusDenied
	a.DecidedAt = &now
	a.DecidedBy = &userID
}

// Repo is the persistence port.
type Repo interface {
	Create(ctx context.Context, a *Approval) error
	Get(ctx context.Context, id string) (*Approval, error)
	ListByIssue(ctx context.Context, issueID string) ([]*Approval, error)
	Update(ctx context.Context, a *Approval) error
	ListPending(ctx context.Context, workspaceID string) ([]*Approval, error)
}

// Gate is the application port that wraps the repo with policy-aware
// decision flow (notify the user, wait for response, return Decision).
// Implementation lands in service/approval/ in R3.x.
type Gate interface {
	Request(ctx context.Context, a *Approval) error
	Wait(ctx context.Context, approvalID string) (Status, error)
	Notify(ctx context.Context, approvalID string, status Status, decidedBy string) error
}