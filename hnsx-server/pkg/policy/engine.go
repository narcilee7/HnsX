// Package policy enforces harness constraints.
package policy

import (
	"fmt"

	"github.com/hnsx-io/hnsx/go/pkg/core"
)

// Engine evaluates policy rules.
type Engine struct {
	policy core.Policy
}

// NewEngine creates a policy engine.
func NewEngine(policy core.Policy) *Engine {
	return &Engine{policy: policy}
}

// CanUseTool checks if a tool is allowed.
func (e *Engine) CanUseTool(toolID string) error {
	for _, denied := range e.policy.DeniedTools {
		if denied == toolID {
			return fmt.Errorf("tool %s is denied by policy", toolID)
		}
	}
	if len(e.policy.AllowedTools) > 0 {
		for _, allowed := range e.policy.AllowedTools {
			if allowed == toolID {
				return nil
			}
		}
		return fmt.Errorf("tool %s is not in allowed list", toolID)
	}
	return nil
}

// CheckBudget checks if a cost exceeds the budget.
func (e *Engine) CheckBudget(costUSD float64) error {
	if e.policy.BudgetUSD > 0 && costUSD > e.policy.BudgetUSD {
		return fmt.Errorf("budget exceeded: %.4f > %.4f", costUSD, e.policy.BudgetUSD)
	}
	return nil
}
