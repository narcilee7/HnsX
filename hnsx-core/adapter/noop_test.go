package adapter_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hnsx-io/hnsx/core/adapter"
	"github.com/hnsx-io/hnsx/core/domain"
)

func TestNoopAdapter_Name(t *testing.T) {
	if got := adapter.NewNoopAdapter().Name(); got != "noop" {
		t.Fatalf("name = %q", got)
	}
	if got := adapter.NewEchoAdapter().Name(); got != "echo" {
		t.Fatalf("name = %q", got)
	}
}

func TestNoopAdapter_Invoke(t *testing.T) {
	a := adapter.NewNoopAdapter()
	out, err := a.Invoke(context.Background(), domain.AgentSpec{
		ID:       "triage",
		Provider: "anthropic",
		Model:    "claude-haiku-4-5",
	}, "system prompt body", map[string]any{"question": "hi"})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.HasPrefix(out, "[noop]") {
		t.Fatalf("output = %q", out)
	}
	for _, want := range []string{"agent=triage", "provider=anthropic", "model=claude-haiku-4-5"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestEchoAdapter_Invoke(t *testing.T) {
	a := adapter.NewEchoAdapter()
	out, err := a.Invoke(context.Background(), domain.AgentSpec{ID: "x"}, "", map[string]any{
		"hello": "world",
	})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(out, "[echo]") || !strings.Contains(out, `"hello":"world"`) {
		t.Fatalf("output = %q", out)
	}
}
