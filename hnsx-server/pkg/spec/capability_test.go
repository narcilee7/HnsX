package spec

import (
	"testing"
)

func TestDeriveCapabilities_IncludesOrchestration(t *testing.T) {
	s := &DomainSpec{
		ID: "d1",
		Harness: HarnessSpec{
			Agents: map[string]AgentSpec{
				"a1": {Provider: "anthropic", Model: "claude", Adapter: AdapterConfig{Kind: "anthropic"}},
			},
			Session: SessionSpec{Mode: Supervisor, Agent: "a1"},
			Sandbox: SandboxSpec{Policy: "none"},
		},
	}
	caps := DeriveCapabilities(s)
	want := map[string]bool{
		"provider:anthropic":  true,
		"model:claude":        true,
		"adapter:anthropic":   true,
		"sandbox:none":        true,
		"orchestration:supervisor": true,
	}
	for _, c := range caps {
		if !want[c] {
			t.Errorf("unexpected capability %q", c)
		}
		delete(want, c)
	}
	if len(want) > 0 {
		for c := range want {
			t.Errorf("missing capability %q", c)
		}
	}
}

func TestDeriveCapabilities_NoOrchestrationWhenEmpty(t *testing.T) {
	s := &DomainSpec{
		ID: "d1",
		Harness: HarnessSpec{
			Agents: map[string]AgentSpec{
				"a1": {Provider: "noop", Adapter: AdapterConfig{Kind: "noop"}},
			},
			Session: SessionSpec{Mode: "", Agent: "a1"},
		},
	}
	caps := DeriveCapabilities(s)
	for _, c := range caps {
		if c == "orchestration:" {
			t.Fatal("expected empty orchestration mode to be omitted")
		}
	}
}
