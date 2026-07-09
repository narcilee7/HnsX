package adapter

import (
	"context"
	"strings"
	"testing"

	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

func TestNoopAdapter(t *testing.T) {
	a := NewNoopAdapter()
	if a.Name() != "noop" {
		t.Fatalf("name = %q", a.Name())
	}
	out, err := a.Invoke(context.Background(), spec.AgentSpec{ID: "x", Provider: "p", Model: "m"}, "sys", map[string]any{"q": "hi"})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(out, "[noop]") {
		t.Fatalf("output = %q", out)
	}
}

func TestEchoAdapter(t *testing.T) {
	a := NewEchoAdapter()
	if a.Name() != "echo" {
		t.Fatalf("name = %q", a.Name())
	}
	out, err := a.Invoke(context.Background(), spec.AgentSpec{ID: "x"}, "sys", map[string]any{"q": "hi"})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if !strings.Contains(out, `"q":"hi"`) {
		t.Fatalf("output = %q", out)
	}
}
