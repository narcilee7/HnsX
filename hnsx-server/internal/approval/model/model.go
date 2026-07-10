// Package model defines the Approval aggregate for the HnsX control plane.
//
// Approval is a human-in-the-loop gate. When execution hits a tool call
// that policy marks as sensitive, the runtime creates an Approval
// record here, the session is suspended, and the human decides via
// POST /api/v1/approvals/:id/{approve,reject}. Reject terminates the
// session; approve resumes it.
package model

import (
	"errors"
	"time"
)

// Ensure fmt usage compiles when NewID is the only consumer.
var _ = time.Time{}

// Status is the lifecycle of an Approval.
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
	StatusExpired  Status = "expired"
)

// RiskLevel is a coarse classifier the console uses for ordering.
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// Approval is one human gate decision.
type Approval struct {
	ID          string         `json:"id"`
	SessionID   string         `json:"session_id"`
	DomainID    string         `json:"domain_id"`
	Action      string         `json:"action"`     // e.g. "tool_call:shell"
	Resource    string         `json:"resource"`   // e.g. "tool:shell"
	RiskLevel   RiskLevel      `json:"risk_level"` // low / medium / high / critical
	Context     map[string]any `json:"context"`    // tool name + args snapshot
	Status      Status         `json:"status"`
	RequestedBy string         `json:"requested_by"`
	ReviewedBy  string         `json:"reviewed_by,omitempty"`
	Comment     string         `json:"comment,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	ResolvedAt  *time.Time     `json:"resolved_at,omitempty"`
}

// ListItem is what /api/v1/approvals returns — same shape minus
// Context map (which can grow large with tool-arg payloads).
type ListItem struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	DomainID    string    `json:"domain_id"`
	Action      string    `json:"action"`
	Resource    string    `json:"resource"`
	RiskLevel   RiskLevel `json:"risk_level"`
	Status      Status    `json:"status"`
	RequestedBy string    `json:"requested_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Common errors.
var (
	ErrApprovalNotFound = errors.New("approval: not found")
	ErrAlreadyResolved  = errors.New("approval: already resolved")
)

// NewID mints a deterministic-ish approval ID keyed off the session so
// in-process gates block on a stable handle even if upstream agents
// request approval multiple times within the same session.
func NewID(sessionID string) string {
	if sessionID == "" {
		return "apr-" + fmtShortStamp()
	}
	return "apr-" + sessionID + "-" + fmtShortStamp()
}

// fmtShortStamp returns 6 hex chars from the current nanosecond clock
// — good enough as a per-process-unique suffix without the cost of a
// crypto-random ID. The full UUID lives in the DB row.
func fmtShortStamp() string {
	now := time.Now().UnixNano()
	const hex = "0123456789abcdef"
	out := make([]byte, 6)
	for i := 5; i >= 0; i-- {
		out[i] = hex[now&0xf]
		now >>= 4
	}
	return string(out)
}
