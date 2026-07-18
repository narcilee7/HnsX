// Package ws implements the WebSocket protocol between the hnsxd
// daemon and the hnsxd server.
//
// Why WS: when the daemon runs on a different host than the server
// (the deployment story after R4), the daemon cannot share the
// server's Postgres pool. The WS protocol gives the daemon a
// transport-agnostic way to claim work, report observations, and
// receive approval requests.
//
// Wire format: newline-delimited JSON over a single WebSocket. Every
// frame is an Envelope; the `Type` field selects the inner payload.
// The server is the authoritative side; the daemon only sends events
// and reads commands.
package ws

import "encoding/json"

// Envelope is the outer wrapper for every WS frame.
type Envelope struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`     // echo back; used for claim round-trip
	Payload json.RawMessage `json:"payload,omitempty"` // schema depends on Type
}

// Envelope type constants.
const (
	// Daemon → server.
	TypeClaim          = "claim"            // daemon asks for assigned issues
	TypeObservations   = "observations"     // batch of observations to persist
	TypeApprovalReply  = "approval_reply"   // daemon's outcome for an approval
	TypeIssueStatus    = "issue_status"     // daemon moved an issue forward / blocked
	TypeHeartbeat      = "heartbeat"        // daemon liveness ping

	// Server → daemon.
	TypeIssues         = "issues"           // response to claim: assigned issues
	TypeApprovalReq    = "approval_request" // server pushes an approval ask to daemon
	TypeAck            = "ack"              // generic acknowledgment
)

// ClaimRequest is the body of a TypeClaim envelope.
type ClaimRequest struct {
	WorkspaceID string `json:"workspace_id"`
	DaemonName  string `json:"daemon_name"`
	MaxItems    int    `json:"max_items"`
}

// IssuesResponse is the body of a TypeIssues envelope.
type IssuesResponse struct {
	Items []ClaimedIssue `json:"items"`
}

// ClaimedIssue mirrors the issue row the daemon needs to act on.
type ClaimedIssue struct {
	ID                 string `json:"id"`
	WorkspaceID        string `json:"workspace_id"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	AssigneeID         string `json:"assignee_id"`
	AgentID            string `json:"agent_id"`
	Backend            string `json:"backend"` // from agent.RuntimeConfig
}

// ObservationEvent is the body of a TypeObservations envelope.
type ObservationEvent struct {
	WorkspaceID     string          `json:"workspace_id"`
	IssueID         string          `json:"issue_id"`
	AgentID         string          `json:"agent_id"`
	Kind            string          `json:"kind"`
	Sequence        int64           `json:"sequence"`
	Payload         json.RawMessage `json:"payload"`
	OccurredAt      string          `json:"occurred_at"` // RFC3339
	PromptHash      string          `json:"prompt_hash"`
	AgentTemplateID string          `json:"agent_template_id"`
	ToolSignatures  json.RawMessage `json:"tool_signatures"`
}

// IssueStatusEvent is the body of a TypeIssueStatus envelope.
type IssueStatusEvent struct {
	IssueID string `json:"issue_id"`
	Status  string `json:"status"`
}

// ApprovalRequestEvent is the body of a TypeApprovalReq envelope.
type ApprovalRequestEvent struct {
	ApprovalID string `json:"approval_id"`
	IssueID    string `json:"issue_id"`
	Action     string `json:"action"`
	Reason     string `json:"reason"`
}

// ApprovalReplyEvent is the body of a TypeApprovalReply envelope.
type ApprovalReplyEvent struct {
	ApprovalID string `json:"approval_id"`
	Decision   string `json:"decision"` // "granted" | "denied"
	Reason     string `json:"reason,omitempty"`
}

// AckResponse is the body of a TypeAck envelope.
type AckResponse struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
}