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
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// Engine evaluates policy rules.
type Engine struct {
	spec             domain.PolicySpec
	turns            atomic.Int64
	promptTokens     atomic.Int64
	completionTokens atomic.Int64
	costUSD          atomic.Uint64 // float64 bits
}

func newEngine(s domain.PolicySpec) *Engine {
	return &Engine{spec: s}
}

// NewEngine constructs a policy engine.
func NewEngine(s domain.PolicySpec) *Engine {
	return newEngine(s)
}

// Spec returns a defensive copy of the underlying policy.
func (e *Engine) Spec() domain.PolicySpec { return e.spec }

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
func (e *Engine) Guardrails() []domain.GuardrailSpec {
	out := make([]domain.GuardrailSpec, len(e.spec.Guardrails))
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

// ErrGuardrailBlocked is returned when a guardrail with action=block fires.
var ErrGuardrailBlocked = errors.New("guardrail blocked")

// GuardrailEvent describes something that happened during a session and may
// trigger a guardrail.
type GuardrailEvent struct {
	Kind    string
	AgentID string
	Text    string
	Payload map[string]any
}

// GuardrailDecision is the result of evaluating a guardrail event.
type GuardrailDecision struct {
	Matched     bool
	Action      string // "block", "log", "human_approval"
	GuardrailID string
	Message     string
}

// EvaluateGuardrails checks the event against every configured guardrail and
// returns the first matching decision. If nothing matches, it returns a
// Matched=false decision with Action="allow".
func (e *Engine) EvaluateGuardrails(event GuardrailEvent) GuardrailDecision {
	for _, g := range e.spec.Guardrails {
		if !guardrailApplies(g, event) {
			continue
		}
		matched, err := guardrailMatches(g, event)
		if err != nil || !matched {
			continue
		}
		action := g.Action
		if action == "" {
			action = "log"
		}
		return GuardrailDecision{
			Matched:     true,
			Action:      action,
			GuardrailID: g.ID,
			Message:     g.Message,
		}
	}
	return GuardrailDecision{Matched: false, Action: "allow"}
}

func guardrailApplies(g domain.GuardrailSpec, event GuardrailEvent) bool {
	if g.On == "" {
		return true
	}
	return g.On == event.Kind
}

func guardrailMatches(g domain.GuardrailSpec, event GuardrailEvent) (bool, error) {
	text := event.Text
	if text == "" && event.Payload != nil {
		if s, ok := event.Payload["content"].(string); ok {
			text = s
		}
	}

	switch g.Type {
	case "contains":
		pattern := guardrailPattern(g)
		return pattern != "" && strings.Contains(text, pattern), nil
	case "regex":
		pattern := guardrailPattern(g)
		if pattern == "" {
			return false, nil
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return false, err
		}
		return re.MatchString(text), nil
	case "json_schema":
		// Phase 1 does not implement schema validation; treat as no-match.
		return false, nil
	default:
		return false, nil
	}
}

func guardrailPattern(g domain.GuardrailSpec) string {
	if g.Schema != "" {
		return g.Schema
	}
	if s, ok := g.Config.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", g.Config)
}

// ----------------------------------------------------------------------------
// float<->bits helpers (atomic-friendly).
// ----------------------------------------------------------------------------

func float64ToBits(f float64) uint64   { return math.Float64bits(f) }
func float64FromBits(b uint64) float64 { return math.Float64frombits(b) }
