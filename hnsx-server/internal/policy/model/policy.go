// Package model defines the Policy aggregate for the HnsX control plane.
package model

import "errors"

// Policy is the persisted policy configuration for a domain.
type Policy struct {
	DomainID    string
	Budget      Budget
	Permissions Permissions
	Guardrails  []Guardrail
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

// Common policy errors.
var (
	ErrPolicyNotFound   = errors.New("policy: not found")
	ErrBudgetExceeded   = errors.New("policy: budget exceeded")
	ErrPermissionDenied = errors.New("policy: permission denied")
	ErrGuardrailBlocked = errors.New("policy: guardrail blocked")
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
