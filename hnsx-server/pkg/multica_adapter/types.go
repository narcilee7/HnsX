// Package multica_adapter exposes Multica's REST + WS API contract on top of
// HnsX's Go control plane. Multica's Next.js frontend, CLI, and daemon talk
// to the adapter unchanged; internally the adapter translates to HnsX's
// services (Domain / Session / Worker / Trace / Approval).
//
// The adapter is the W1 deliverable that makes Multica a usable frontend for
// HarnessX. Domain / Harness / Policy / Approval / Eval additions arrive in
// later roadmap phases on top of this contract.
package multica_adapter

import (
	"encoding/json"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────
// Common JSON shapes (Multica wire format)
// ────────────────────────────────────────────────────────────────────────────

// UserResponse mirrors Multica's "user" representation.
type UserResponse struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Email     string  `json:"email"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

// WorkspaceResponse mirrors Multica's GET /api/workspaces/:id.
type WorkspaceResponse struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description string          `json:"description,omitempty"`
	Context     string          `json:"context,omitempty"`
	Settings    json.RawMessage `json:"settings"`
	CreatedAt   string          `json:"created_at"`
	UpdatedAt   string          `json:"updated_at"`
}

// MemberResponse mirrors Multica's GET /api/workspaces/:id/members.
type MemberResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	UserID      string `json:"user_id"`
	Role        string `json:"role"`
	CreatedAt   string `json:"created_at"`
}

// AgentResponse mirrors Multica's GET /api/workspaces/:id/agents/:agentId.
//
// Multica's "agent" is a virtual teammate; HnsX stores this as a Domain with
// mode=single. The adapter translates between the two on the fly.
type AgentResponse struct {
	ID                string          `json:"id"`
	WorkspaceID       string          `json:"workspace_id"`
	Name              string          `json:"name"`
	AvatarURL         *string         `json:"avatar_url,omitempty"`
	RuntimeMode       string          `json:"runtime_mode"` // "local" | "cloud"
	RuntimeConfig     json.RawMessage `json:"runtime_config"`
	Visibility        string          `json:"visibility"` // "workspace" | "private"
	Status            string          `json:"status"`     // "idle" | "working" | "blocked" | "error" | "offline"
	MaxConcurrentTasks int            `json:"max_concurrent_tasks"`
	OwnerID           *string         `json:"owner_id,omitempty"`
	ArchivedAt        *string         `json:"archived_at,omitempty"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
}

// IssueResponse mirrors Multica's GET /api/issues/:id.
//
// Multica's "issue" is a board card that an agent works on; HnsX stores this
// as a Session triggered against a Domain. Issue.status maps to Session.state.
type IssueResponse struct {
	ID                string          `json:"id"`
	WorkspaceID       string          `json:"workspace_id"`
	Title             string          `json:"title"`
	Description       string          `json:"description,omitempty"`
	Status            string          `json:"status"`     // "backlog" | "todo" | "in_progress" | "in_review" | "done" | "blocked" | "cancelled"
	Priority          string          `json:"priority"`   // "urgent" | "high" | "medium" | "low" | "none"
	AssigneeType      *string         `json:"assignee_type,omitempty"` // "member" | "agent"
	AssigneeID        *string         `json:"assignee_id,omitempty"`
	CreatorType       string          `json:"creator_type"` // "member" | "agent"
	CreatorID         string          `json:"creator_id"`
	ParentIssueID     *string         `json:"parent_issue_id,omitempty"`
	AcceptanceCriteria json.RawMessage `json:"acceptance_criteria"`
	ContextRefs       json.RawMessage `json:"context_refs"`
	Position          float64         `json:"position"`
	DueDate           *string         `json:"due_date,omitempty"`
	Number            int             `json:"number"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
}

// CommentResponse mirrors Multica's GET /api/issues/:id/comments.
//
// Multica's "comment" carries agent conversation, status changes, progress
// updates. HnsX emits an Observation for each.
type CommentResponse struct {
	ID        string  `json:"id"`
	IssueID   string  `json:"issue_id"`
	AuthorType string  `json:"author_type"` // "member" | "agent"
	AuthorID  string  `json:"author_id"`
	Content   string  `json:"content"`
	Type      string  `json:"type"` // "comment" | "status_change" | "progress_update" | "system"
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// SquadResponse mirrors Multica's GET /api/squads/:id.
//
// HnsX stores this as a Domain with mode=supervisor or mode=workflow. The
// adapter translates the leader + members map to HarnessX's specialists.
type SquadResponse struct {
	ID            string                       `json:"id"`
	WorkspaceID   string                       `json:"workspace_id"`
	Name          string                       `json:"name"`
	Description   string                       `json:"description,omitempty"`
	Instructions  string                       `json:"instructions,omitempty"`
	AvatarURL     *string                      `json:"avatar_url,omitempty"`
	LeaderID      string                       `json:"leader_id"`
	CreatorID     string                       `json:"creator_id"`
	CreatedAt     string                       `json:"created_at"`
	UpdatedAt     string                       `json:"updated_at"`
	ArchivedAt    *string                      `json:"archived_at,omitempty"`
	ArchivedBy    *string                      `json:"archived_by,omitempty"`
	MemberCount   int                          `json:"member_count"`
	MemberPreview []SquadMemberPreviewResponse `json:"member_preview"`
}

// SquadMemberPreviewResponse is the inline preview shown in list responses.
type SquadMemberPreviewResponse struct {
	MemberType string `json:"member_type"`
	MemberID   string `json:"member_id"`
	Role       string `json:"role"`
}

// AgentRuntimeResponse mirrors Multica's runtime registration entity.
type AgentRuntimeResponse struct {
	ID         string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	DaemonID   string `json:"daemon_id"`
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	Status     string `json:"status"` // "online" | "offline"
	DeviceInfo string `json:"device_info"`
	LastSeenAt *string `json:"last_seen_at,omitempty"`
}

// AgentTaskResponse mirrors Multica's GET /api/daemon/runtimes/:id/tasks/claim
// response. The daemon consumes this to render the task brief and spawn the
// agent subprocess.
//
// In the HarnessX adapter, this maps 1:1 onto an HnsX Session: the session's
// domain_id is the agent, its trigger is the brief, and its state drives the
// status field.
type AgentTaskResponse struct {
	ID                string         `json:"id"`
	AgentID           string         `json:"agent_id"`
	RuntimeID         string         `json:"runtime_id"`
	IssueID           string         `json:"issue_id"`
	WorkspaceID       string         `json:"workspace_id"`
	WorkspaceContext  string         `json:"workspace_context,omitempty"`
	ThreadName        string         `json:"thread_name,omitempty"`
	Status            string         `json:"status"`
	Priority          int32          `json:"priority"`
	DispatchedAt      *string        `json:"dispatched_at"`
	StartedAt         *string        `json:"started_at"`
	CompletedAt       *string        `json:"completed_at"`
	Result            any            `json:"result"`
	Error             *string        `json:"error"`
	FailureReason     string         `json:"failure_reason,omitempty"`
	Attempt           int32          `json:"attempt"`
	MaxAttempts       int32          `json:"max_attempts"`
	IsLeaderTask      bool           `json:"is_leader_task,omitempty"`
	// Agent carries the agent descriptor embedded in the task brief so the
	// daemon doesn't need a second roundtrip.
	Agent *TaskAgentData `json:"agent,omitempty"`
	// Trigger carries the issue title/description that the daemon renders as
	// the agent's opening prompt.
	Trigger map[string]any `json:"trigger,omitempty"`
	// BriefSummary is a pre-rendered summary the daemon shows to operators.
	BriefSummary string `json:"brief_summary,omitempty"`
}

// TaskAgentData is the agent descriptor embedded in an AgentTaskResponse.
type TaskAgentData struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// ────────────────────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────────────────────

// nowISO returns the canonical RFC3339 timestamp Multica uses for created_at
// / updated_at fields.
func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}
