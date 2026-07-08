// Package adapter provides adapters for external agents.
package adapter

import (
	"context"
	"fmt"

	"github.com/hnsx-io/hnsx/go/pkg/core"
)

// NoopAdapter is a no-op adapter for testing and local development.
type NoopAdapter struct{}

// NewNoopAdapter creates a new NoopAdapter.
func NewNoopAdapter() *NoopAdapter {
	return &NoopAdapter{}
}

// Invoke returns a deterministic response for testing.
func (a *NoopAdapter) Invoke(ctx context.Context, agent core.Agent, prompt string, input map[string]interface{}) (string, error) {
	return fmt.Sprintf("[noop] agent=%s provider=%s model=%s prompt_len=%d input_keys=%d",
		agent.ID, agent.Model.Provider, agent.Model.Model, len(prompt), len(input)), nil
}

// EchoAdapter echoes the input as output for testing.
type EchoAdapter struct{}

// NewEchoAdapter creates a new EchoAdapter.
func NewEchoAdapter() *EchoAdapter {
	return &EchoAdapter{}
}

// Invoke echoes the input.
func (a *EchoAdapter) Invoke(ctx context.Context, agent core.Agent, prompt string, input map[string]interface{}) (string, error) {
	return fmt.Sprintf("[echo] agent=%s input=%v", agent.ID, input), nil
}
