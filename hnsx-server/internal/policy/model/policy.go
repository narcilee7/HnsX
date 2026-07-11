// Package model defines the Policy aggregate for the HnsX control plane.
//
// A Policy is a NAMED bundle (Budget + Permissions + Guardrails) that can
// be bound to a Domain. The same Policy.ID can be referenced by multiple
// domains in the future; today we keep a single binding via the
// domain_uuid column on the `policies` table.
package model

import (
	"errors"
	"time"
)

// Policy is the persisted named policy.
type Policy struct {
	ID          string      // unique per tenant — the user-facing handle
	Name        string      // human-readable label
	Description string      // optional operator note
	Budget      Budget      // budget caps
	Permissions Permissions // capability toggles
	Guardrails  []Guardrail // runtime hooks
	BoundDomain string      // optional: domain_id this policy is bound to
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Budget defines cost/turn/token limits.
type Budget struct {
	MaxCostUSD float64
	MaxTurns   int
	MaxTokens  int
}

// Permissions toggles dangerous capabilities.
type Permissions struct {
	AllowFileWrite  bool
	AllowFileDelete bool
	AllowNetwork    bool
	AllowShell      bool
}

// Guardrail defines a runtime check.
type Guardrail struct {
	ID      string
	Type    string
	On      string
	Action  string
	Schema  string
	Message string
	Config  any
}

// Rules is the JSONB-friendly projection of (Budget, Permissions,
// Guardrails) used when persisting into Postgres.
type Rules struct {
	Budget      Budget      `json:"budget"`
	Permissions Permissions `json:"permissions"`
	Guardrails  []Guardrail `json:"guardrails"`
}

// ListItem is what /api/v1/policies returns — enough metadata to drive
// the Settings console without dragging in the rule body if it's large.
type ListItem struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	BoundDomain string      `json:"bound_domain,omitempty"`
	Budget      Budget      `json:"budget"`
	Permissions Permissions `json:"permissions"`
	Guardrails  []Guardrail `json:"guardrails"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}

// Common policy errors.
var (
	ErrPolicyNotFound   = errors.New("policy: not found")
	ErrBudgetExceeded   = errors.New("policy: budget exceeded")
	ErrPermissionDenied = errors.New("policy: permission denied")
	ErrGuardrailBlocked = errors.New("policy: guardrail blocked")
	ErrInvalidPolicyID  = errors.New("policy: invalid id")
)

// Event describes something that happened during a session and may trigger
// policy enforcement or audit logging.
type Event struct {
	SessionID string
	DomainID  string
	Kind      string // "tool_call", "adapter_invoke", "file_write", ...
	CostUSD   float64
	Tokens    int
}
