// Package adapter provides implementations of the runtime.Adapter contract.
// Adapters wrap an external Agent model so that the runtime stays decoupled
// from any specific provider. Two adapters are shipped here:
//
//   - NoopAdapter: deterministic, offline-friendly response. Used for tests
//     and for the "verify the harness pipeline without any LLM" smoke path.
//   - EchoAdapter: echoes the trigger back. Useful for UI demos.
package adapter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// Compile-time checks: both adapters implement runtime.Adapter.
var (
	_ runtime.Adapter = (*NoopAdapter)(nil)
	_ runtime.Adapter = (*EchoAdapter)(nil)
)

// NoopAdapter returns a deterministic, provider-agnostic response. Real
// adapters (anthropic, openai, etc.) live in their own subpackages.
type NoopAdapter struct{}

// NewNoopAdapter constructs a NoopAdapter.
func NewNoopAdapter() *NoopAdapter { return &NoopAdapter{} }

// Name returns "noop".
func (a *NoopAdapter) Name() string { return "noop" }

// Invoke produces a deterministic echo of the agent identity, prompt length
// and input key set.
func (a *NoopAdapter) Invoke(_ context.Context, agent spec.AgentSpec, prompt string, input map[string]any) (string, error) {
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	return fmt.Sprintf("[noop] agent=%s provider=%s model=%s prompt_len=%d input_keys=%v",
		agent.ID, agent.Provider, agent.Model, len(prompt), keys), nil
}

// EchoAdapter echoes the input map back as a JSON string.
type EchoAdapter struct{}

// NewEchoAdapter constructs an EchoAdapter.
func NewEchoAdapter() *EchoAdapter { return &EchoAdapter{} }

// Name returns "echo".
func (a *EchoAdapter) Name() string { return "echo" }

// Invoke returns a JSON dump of the input map alongside a header.
func (a *EchoAdapter) Invoke(_ context.Context, agent spec.AgentSpec, _ string, input map[string]any) (string, error) {
	body, _ := json.Marshal(input)
	return fmt.Sprintf("[echo] agent=%s input=%s", agent.ID, string(body)), nil
}
