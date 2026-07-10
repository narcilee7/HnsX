// Package policy enforces harness constraints declared in
// domain.PolicySpec. The runtime invokes Engine at every Tool call, before
// each cost-bearing Adapter invocation, and when a guardrail event fires.
//
// Phase 1 covers:
//
//   - Budget enforcement (max cost / max turns / max tokens).
//   - Capability gating via PermissionSpec (file/network/shell).
//   - Guardrail listing — the actual guardrail logic is interpreted at the
//     point of use; Engine only validates that each guardrail is well-formed.
package policy

import (
	"errors"
	"fmt"
	"math"
	"sync/atomic"

	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// Engine evaluates policy rules.
type Engine struct {
	spec             spec.PolicySpec
	turns            atomic.Int64
	promptTokens     atomic.Int64
	completionTokens atomic.Int64
	costUSD          atomic.Uint64 // float64 bits
}

func newEngine(s spec.PolicySpec) *Engine {
	return &Engine{spec: s}
}

// NewEngine constructs a policy engine.
func NewEngine(s spec.PolicySpec) *Engine {
	return newEngine(s)
}

// Spec returns a defensive copy of the underlying policy.
func (e *Engine) Spec() spec.PolicySpec { return e.spec }

// ErrBudgetExceeded is returned when an invocation would breach the budget.
var ErrBudgetExceeded = errors.New("budget exceeded")

// ErrPermissionDenied is returned when a tool category is gated by policy.
var ErrPermissionDenied = errors.New("permission denied")

// CheckBudget returns an error if adding cost would exceed the budget.
// A non-positive MaxCostUSD disables cost gating.
func (e *Engine) CheckBudget(costUSD float64) error {
	if e.spec.Budget.MaxCostUSD <= 0 {
		return nil
	}
	current := float64FromBits(e.costUSD.Load())
	if current+costUSD > e.spec.Budget.MaxCostUSD {
		return fmt.Errorf("%w: %.6f + %.6f > %.6f",
			ErrBudgetExceeded, current, costUSD, e.spec.Budget.MaxCostUSD)
	}
	return nil
}

// RecordCost atomically updates the cost counter. Call this after each
// successful Adapter invocation so future CheckBudget calls include the spend.
func (e *Engine) RecordCost(costUSD float64) {
	if costUSD <= 0 {
		return
	}
	current := float64FromBits(e.costUSD.Load())
	e.costUSD.Store(float64ToBits(current + costUSD))
}

// RecordTokens accumulates token counts against the budget.
func (e *Engine) RecordTokens(prompt, completion int) {
	if prompt > 0 {
		e.promptTokens.Add(int64(prompt))
	}
	if completion > 0 {
		e.completionTokens.Add(int64(completion))
	}
	e.turns.Add(1)
}

// CheckTurns returns ErrBudgetExceeded if the session has reached its turn cap.
// A non-positive MaxTurns disables turn gating.
func (e *Engine) CheckTurns() error {
	if e.spec.Budget.MaxTurns <= 0 {
		return nil
	}
	if e.turns.Load() >= int64(e.spec.Budget.MaxTurns) {
		return fmt.Errorf("%w: turns=%d >= max=%d",
			ErrBudgetExceeded, e.turns.Load(), e.spec.Budget.MaxTurns)
	}
	return nil
}

// CheckTokens returns ErrBudgetExceeded if total tokens exceed the cap.
// A non-positive MaxTokens disables token gating.
func (e *Engine) CheckTokens() error {
	if e.spec.Budget.MaxTokens <= 0 {
		return nil
	}
	total := e.promptTokens.Load() + e.completionTokens.Load()
	if total >= int64(e.spec.Budget.MaxTokens) {
		return fmt.Errorf("%w: tokens=%d >= max=%d",
			ErrBudgetExceeded, total, e.spec.Budget.MaxTokens)
	}
	return nil
}

// CanUseFileWrite / CanUseFileDelete / CanUseNetwork / CanUseShell each return
// nil when the corresponding capability is granted. The default when no
// permission is declared is to allow (preserves intent: explicitly denied
// capabilities are named).
func (e *Engine) CanUseFileWrite() error {
	if !e.spec.Permissions.AllowFileWrite {
		return fmt.Errorf("%w: file_write", ErrPermissionDenied)
	}
	return nil
}

// CanUseFileDelete reports whether explicit file deletion is allowed.
func (e *Engine) CanUseFileDelete() error {
	if !e.spec.Permissions.AllowFileDelete {
		return fmt.Errorf("%w: file_delete", ErrPermissionDenied)
	}
	return nil
}

// CanUseNetwork reports whether outbound network is allowed.
func (e *Engine) CanUseNetwork() error {
	if !e.spec.Permissions.AllowNetwork {
		return fmt.Errorf("%w: network", ErrPermissionDenied)
	}
	return nil
}

// CanUseShell reports whether shell execution is allowed.
func (e *Engine) CanUseShell() error {
	if !e.spec.Permissions.AllowShell {
		return fmt.Errorf("%w: shell", ErrPermissionDenied)
	}
	return nil
}

// Guardrails returns the configured guardrails in order.
func (e *Engine) Guardrails() []spec.GuardrailSpec {
	out := make([]spec.GuardrailSpec, len(e.spec.Guardrails))
	copy(out, e.spec.Guardrails)
	return out
}

// Snapshot returns the current cost/token/turn counters as a runtime.Cost
// value. This is the single source of truth used to populate
// runtime.Observation.Cost and drive trace/metric aggregation.
func (e *Engine) Snapshot() *runtime.Cost {
	return &runtime.Cost{
		PromptTokens:     int(e.promptTokens.Load()),
		CompletionTokens: int(e.completionTokens.Load()),
		CostUSD:          float64FromBits(e.costUSD.Load()),
	}
}

// ----------------------------------------------------------------------------
// float<->bits helpers (atomic-friendly).
// ----------------------------------------------------------------------------

func float64ToBits(f float64) uint64   { return math.Float64bits(f) }
func float64FromBits(b uint64) float64 { return math.Float64frombits(b) }
