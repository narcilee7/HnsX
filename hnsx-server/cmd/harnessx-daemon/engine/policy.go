// Package engine hosts the HarnessX in-process engine: domain resolution,
// skill loading, policy checks, approval flow, evaluation, and memory.
//
// P0 (W6) keeps this minimal: a single Check function that gates a session
// against cost / permission / approval rules before the agent subprocess
// is allowed to run. Later phases grow skill resolver, eval runner, and
// memory backend.
package engine

import (
	"context"
	"fmt"
)

// Decision is the result of a policy check.
type Decision struct {
	// Allow indicates the action is permitted without further gating.
	Allow bool
	// RequireApproval means the action is gated by a human review.
	RequireApproval bool
	// EstimatedCostUSD is the projected spend if the action runs.
	EstimatedCostUSD float64
	// Reason explains why the decision was made.
	Reason string
}

// Policy is the interface every policy backend implements. W6 ships a
// flat-rules implementation; later phases may swap in a richer engine.
type Policy interface {
	Check(ctx context.Context, action Action) Decision
}

// Action describes the operation a session wants to perform.
type Action struct {
	// DomainID identifies the DomainSpec whose policy applies.
	DomainID string
	// AgentID is the resolved agent that will run the action.
	AgentID string
	// Kind names the operation: "session.start", "tool.invoke", etc.
	Kind string
	// EstimatedCostUSD is the projected spend (callers populate from the
	// model's per-call estimate).
	EstimatedCostUSD float64
	// Resource is the optional resource the action targets (file path,
	// URL, command name). Empty when not applicable.
	Resource string
}

// FlatPolicy is a static-rules policy. It is the W6 reference implementation
// and is replaced in W12 by the HnsX server's full PolicySpec engine.
type FlatPolicy struct {
	// MaxCostUSD is the hard ceiling for one session. Sessions whose
	// projected cost exceeds this trigger approval.
	MaxCostUSD float64
	// BlockedResources is the deny-list of resource patterns.
	BlockedResources []string
}

// Check implements Policy.
func (p *FlatPolicy) Check(ctx context.Context, a Action) Decision {
	if p == nil {
		return Decision{Allow: true, Reason: "no policy configured"}
	}
	for _, blocked := range p.BlockedResources {
		if a.Resource != "" && blocked == a.Resource {
			return Decision{Allow: false, Reason: fmt.Sprintf("resource blocked: %s", blocked)}
		}
	}
	if p.MaxCostUSD > 0 && a.EstimatedCostUSD > p.MaxCostUSD {
		return Decision{
			RequireApproval: true,
			EstimatedCostUSD: a.EstimatedCostUSD,
			Reason:          fmt.Sprintf("cost %.4f exceeds max %.4f", a.EstimatedCostUSD, p.MaxCostUSD),
		}
	}
	return Decision{Allow: true, EstimatedCostUSD: a.EstimatedCostUSD}
}
