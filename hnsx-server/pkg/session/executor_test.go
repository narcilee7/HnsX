package session

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/adapter"
	"github.com/hnsx-io/hnsx/server/pkg/policy"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

func TestExecutor_Execute_Permissive(t *testing.T) {
	exec := NewExecutor(adapter.NewEchoAdapter())
	ds := &spec.DomainSpec{
		ID: "d-permissive",
		Harness: spec.HarnessSpec{
			Agents: map[string]spec.AgentSpec{
				"a": {Provider: "echo", Adapter: spec.AdapterConfig{Kind: "echo"}},
			},
			Session: spec.SessionSpec{Mode: spec.Single, Agent: "a"},
		},
	}

	result, err := exec.Execute(context.Background(), ds, map[string]any{"msg": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.State != "completed" {
		t.Fatalf("expected completed, got %s", result.State)
	}
}

func TestExecutor_Execute_BudgetTurnsBlocks(t *testing.T) {
	provider := &staticPolicyProvider{
		engine: policy.NewEngine(spec.PolicySpec{
			Budget: spec.BudgetSpec{MaxTurns: 1},
		}),
	}

	recorder := &collectingRecorder{}
	exec := NewExecutor(adapter.NewEchoAdapter()).
		WithPolicyProvider(provider).
		WithAuditRecorder(recorder)

	ds := &spec.DomainSpec{
		ID: "d-budget",
		Harness: spec.HarnessSpec{
			Agents: map[string]spec.AgentSpec{
				"a": {Provider: "echo", Adapter: spec.AdapterConfig{Kind: "echo"}},
			},
			Session: spec.SessionSpec{Mode: spec.Single, Agent: "a"},
		},
	}

	// First run consumes the single allowed turn.
	_, err := exec.Execute(context.Background(), ds, map[string]any{"msg": "first"})
	if err != nil {
		t.Fatalf("first run should succeed: %v", err)
	}

	// Second run should be blocked because the shared provider returns the same
	// engine with one turn already recorded.
	_, err = exec.Execute(context.Background(), ds, map[string]any{"msg": "second"})
	if err == nil {
		t.Fatal("expected budget exceeded error")
	}
	if !errors.Is(err, policy.ErrBudgetExceeded) {
		t.Fatalf("expected ErrBudgetExceeded, got %v", err)
	}

	if len(recorder.entries) == 0 {
		t.Fatal("expected audit entries")
	}
}

func TestExecutor_Execute_PermissionDenied(t *testing.T) {
	provider := &staticPolicyProvider{
		engine: policy.NewEngine(spec.PolicySpec{
			Permissions: spec.PermissionSpec{AllowFileWrite: false},
		}),
	}
	exec := NewExecutor(adapter.NewEchoAdapter()).WithPolicyProvider(provider)

	ds := &spec.DomainSpec{
		ID: "d-perm",
		Harness: spec.HarnessSpec{
			Agents: map[string]spec.AgentSpec{
				"a": {
					Provider: "echo",
					Adapter:  spec.AdapterConfig{Kind: "echo"},
					Tools:    []string{"writer"},
				},
			},
			Tools: map[string]spec.ToolConfig{
				"writer": {Kind: "file_write"},
			},
			Session: spec.SessionSpec{Mode: spec.Single, Agent: "a"},
		},
	}

	_, err := exec.Execute(context.Background(), ds, map[string]any{"msg": "x"})
	if err == nil {
		t.Fatal("expected permission denied error")
	}
	if !errors.Is(err, policy.ErrPermissionDenied) {
		t.Fatalf("expected ErrPermissionDenied, got %v", err)
	}
}

type staticPolicyProvider struct {
	engine *policy.Engine
}

func (p *staticPolicyProvider) SessionEngine(_, _ string) (*policy.Engine, error) {
	return p.engine, nil
}

type collectingRecorder struct {
	entries []AuditEntry
}

func (r *collectingRecorder) Record(_ context.Context, entry AuditEntry) error {
	r.entries = append(r.entries, entry)
	return nil
}

func TestExecutor_Execute_AttachesCostToObservations(t *testing.T) {
	sink := &collectingSink{done: make(chan runtime.Observation, 1)}
	provider := &staticPolicyProvider{
		engine: policy.NewEngine(spec.PolicySpec{
			Budget: spec.BudgetSpec{MaxCostUSD: 10.0},
		}),
	}
	exec := NewExecutor(adapter.NewEchoAdapter(), sink).
		WithPolicyProvider(provider)

	ds := &spec.DomainSpec{
		ID: "d-cost",
		Harness: spec.HarnessSpec{
			Agents: map[string]spec.AgentSpec{
				"a": {Provider: "echo", Adapter: spec.AdapterConfig{Kind: "echo"}},
			},
			Session: spec.SessionSpec{Mode: spec.Single, Agent: ""},
		},
	}

	_, err := exec.Execute(context.Background(), ds, map[string]any{"msg": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case obs := <-sink.done:
		if obs.Cost == nil {
			t.Fatal("expected agent_text observation to have a cost snapshot")
		}
		if obs.Cost.CompletionTokens == 0 {
			t.Fatal("expected non-zero completion tokens on cost snapshot")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for agent_text observation")
	}
}

type collectingSink struct {
	mu           sync.Mutex
	observations []runtime.Observation
	done         chan runtime.Observation
}

func (s *collectingSink) Name() string { return "collecting" }
func (s *collectingSink) Record(_ context.Context, obs runtime.Observation) error {
	s.mu.Lock()
	s.observations = append(s.observations, obs)
	s.mu.Unlock()
	if obs.Kind == "agent_text" {
		select {
		case s.done <- obs:
		default:
		}
	}
	return nil
}
func (s *collectingSink) Flush(context.Context) error { return nil }
func (s *collectingSink) Close(context.Context) error { return nil }

func TestExecutor_Execute_GuardrailBlocks(t *testing.T) {
	provider := &staticPolicyProvider{
		engine: policy.NewEngine(spec.PolicySpec{
			Guardrails: []spec.GuardrailSpec{
				{
					ID:      "no-password",
					Type:    "contains",
					On:      "agent_text",
					Action:  "block",
					Config:  "password",
					Message: "output contains a secret",
				},
			},
		}),
	}
	exec := NewExecutor(adapter.NewEchoAdapter()).WithPolicyProvider(provider)

	ds := &spec.DomainSpec{
		ID: "d-guardrail",
		Harness: spec.HarnessSpec{
			Agents: map[string]spec.AgentSpec{
				"a": {Provider: "echo", Adapter: spec.AdapterConfig{Kind: "echo"}},
			},
			Session: spec.SessionSpec{Mode: spec.Single, Agent: "a"},
		},
	}

	_, err := exec.Execute(context.Background(), ds, map[string]any{"msg": "the password is secret"})
	if err == nil {
		t.Fatal("expected guardrail block error")
	}
	if !errors.Is(err, policy.ErrGuardrailBlocked) {
		t.Fatalf("expected ErrGuardrailBlocked, got %v", err)
	}
}

// Ensure staticPolicyProvider implements the interface.
var _ PolicyEngineProvider = (*staticPolicyProvider)(nil)

// Ensure collectingRecorder implements the interface.
var _ AuditRecorder = (*collectingRecorder)(nil)

// Ensure noopRecorder implements the interface.
var _ AuditRecorder = noopRecorder{}

// Ensure permissiveProvider implements the interface.
var _ PolicyEngineProvider = permissiveProvider{}

// Ensure policyAdapter implements runtime.Adapter.
var _ runtime.Adapter = (*policyAdapter)(nil)
