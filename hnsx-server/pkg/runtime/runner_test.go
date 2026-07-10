package runtime

import (
	"context"
	"strings"
	"testing"

	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

type captureSink struct {
	NameVal string
	calls   []Observation
}

func (c *captureSink) Name() string { return c.NameVal }
func (c *captureSink) Record(_ context.Context, obs Observation) error {
	c.calls = append(c.calls, obs)
	return nil
}
func (c *captureSink) Flush(_ context.Context) error { return nil }
func (c *captureSink) Close(_ context.Context) error { return nil }

type stubAdapter struct {
	out string
}

func (s *stubAdapter) Name() string { return "stub" }
func (s *stubAdapter) Invoke(_ context.Context, _ spec.AgentSpec, _ string, _ map[string]any) (string, error) {
	return s.out, nil
}

func TestRunner_SingleMode_HappyPath(t *testing.T) {
	s := &spec.DomainSpec{
		ID: "test", Version: "0.1.0",
		Harness: spec.HarnessSpec{
			Agents: map[string]spec.AgentSpec{
				"main": {ID: "main", Provider: "anthropic", Model: "m"},
			},
			Session: spec.SessionSpec{Mode: spec.Single, Agent: "main"},
		},
	}

	runner := NewRunner(&stubAdapter{out: "hello"})

	var captured []Observation
	runner.WithHook(func(o Observation) { captured = append(captured, o) })

	res, err := runner.Run(context.Background(), s, map[string]any{"question": "hi"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.State != "completed" {
		t.Fatalf("state = %q", res.State)
	}
	if res.Output["response"] != "hello" {
		t.Fatalf("output.response = %v", res.Output["response"])
	}

	// Expect at least: session_start, agent_invoke, agent_text, session_end.
	want := map[string]bool{
		"session_start": false,
		"agent_invoke":  false,
		"agent_text":    false,
		"session_end":   false,
	}
	for _, obs := range captured {
		if _, ok := want[obs.Kind]; ok {
			want[obs.Kind] = true
		}
	}
	for k, v := range want {
		if !v {
			t.Errorf("missing observation kind %q", k)
		}
	}
}

func TestRunner_WorkflowMode_WalksSteps(t *testing.T) {
	s := &spec.DomainSpec{
		ID: "wf", Version: "0.1.0",
		Harness: spec.HarnessSpec{
			Agents: map[string]spec.AgentSpec{
				"a": {ID: "a", Provider: "p", Model: "m"},
				"b": {ID: "b", Provider: "p", Model: "m"},
			},
			Session: spec.SessionSpec{
				Mode: spec.Workflow,
				Workflow: &spec.WorkflowSpec{
					Entry: "s1",
					Steps: []spec.StepSpec{
						{ID: "s1", Agent: "a", Output: "x", Next: "s2"},
						{ID: "s2", Agent: "b", Input: map[string]any{"x": "${x}"}},
					},
				},
			},
		},
	}

	calls := 0
	stub := &countingAdapter{count: &calls}
	runner := NewRunner(stub).WithHook(func(o Observation) {})

	res, err := runner.Run(context.Background(), s, map[string]any{"trigger": "1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.State != "completed" {
		t.Fatalf("state = %q", res.State)
	}
	if calls != 2 {
		t.Errorf("invocations = %d, want 2", calls)
	}
}

type countingAdapter struct{ count *int }

func (c *countingAdapter) Name() string { return "count" }
func (c *countingAdapter) Invoke(_ context.Context, _ spec.AgentSpec, _ string, in map[string]any) (string, error) {
	*c.count++
	return "ok", nil
}

func TestRunner_UnknownMode_ReturnsError(t *testing.T) {
	s := &spec.DomainSpec{
		ID: "x", Version: "0.1.0",
		Harness: spec.HarnessSpec{
			Agents:  map[string]spec.AgentSpec{"a": {ID: "a", Provider: "p", Model: "m"}},
			Session: spec.SessionSpec{Mode: "sorcerer"},
		},
	}
	_, err := NewRunner(&stubAdapter{}).Run(context.Background(), s, nil)
	if err == nil || !strings.Contains(err.Error(), "unknown session mode") {
		t.Fatalf("expected unknown mode error, got %v", err)
	}
}

func TestRunner_SupervisorMode_NotImplemented(t *testing.T) {
	s := &spec.DomainSpec{
		ID: "x", Version: "0.1.0",
		Harness: spec.HarnessSpec{
			Agents:  map[string]spec.AgentSpec{"a": {ID: "a", Provider: "p", Model: "m"}},
			Session: spec.SessionSpec{Mode: spec.Supervisor},
		},
	}
	_, err := NewRunner(&stubAdapter{}).Run(context.Background(), s, nil)
	if err == nil || !strings.Contains(err.Error(), "not yet implemented") {
		t.Fatalf("expected not-implemented error, got %v", err)
	}
}
