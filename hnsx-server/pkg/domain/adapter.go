// Adapter contract — moved from pkg/runtime/runner.go in Phase 3.
// Kept as a contract only; concrete implementations (noop/echo/anthropic/
// openai/...) live in pkg/adapter/* and the Python worker.

package domain

import "context"

// Adapter is the contract for invoking an external Agent.
// Implementations live in pkg/adapter and MUST be safe for concurrent use.
type Adapter interface {
	// Name returns the adapter kind (e.g. "noop", "echo", "anthropic").
	Name() string
	// Invoke calls the underlying agent and returns its text reply.
	Invoke(ctx context.Context, agent AgentSpec, prompt string, input map[string]any) (string, error)
}
