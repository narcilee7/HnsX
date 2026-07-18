package policy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hnsx-io/hnsx/server/internal/domain/policy"
)

func TestEngine_DefaultAllow(t *testing.T) {
	e := NewEngine()
	p := &policy.Policy{ID: "p1", Name: "test", Rules: json.RawMessage("[]")}
	d, err := e.Evaluate(context.Background(), p, policy.EvalContext{CostUSD: 0.01, TokensIn: 10})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if d.Action != policy.ActionAllow {
		t.Errorf("default action = %q, want allow", d.Action)
	}
}

func TestEngine_CostCeiling(t *testing.T) {
	e := NewEngine()
	rules := json.RawMessage(`[
		{"id":"r1","kind":"cost","expression":"cost_usd > 1.0","action":"deny","priority":0}
	]`)
	p := &policy.Policy{ID: "p1", Name: "test", Rules: rules}

	cases := []struct {
		cost float64
		want policy.Action
	}{
		{0.5, policy.ActionAllow},
		{1.5, policy.ActionDeny},
	}
	for _, c := range cases {
		d, err := e.Evaluate(context.Background(), p, policy.EvalContext{CostUSD: c.cost})
		if err != nil {
			t.Fatalf("evaluate cost=%v: %v", c.cost, err)
		}
		if d.Action != c.want {
			t.Errorf("cost=%v: action=%q, want %q", c.cost, d.Action, c.want)
		}
	}
}

func TestEngine_TokensInCeiling(t *testing.T) {
	e := NewEngine()
	rules := json.RawMessage(`[
		{"id":"r1","kind":"cost","expression":"tokens_in > 1000","action":"approval_required","priority":0}
	]`)
	p := &policy.Policy{ID: "p1", Name: "test", Rules: rules}

	d, err := e.Evaluate(context.Background(), p, policy.EvalContext{TokensIn: 1500})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if d.Action != policy.ActionApprovalRequired {
		t.Errorf("tokens_in=1500: action=%q, want approval_required", d.Action)
	}
}

func TestEngine_PriorityFirstMatch(t *testing.T) {
	e := NewEngine()
	rules := json.RawMessage(`[
		{"id":"r1","kind":"cost","expression":"cost_usd > 0","action":"deny","priority":0},
		{"id":"r2","kind":"cost","expression":"cost_usd > 100","action":"approval_required","priority":1}
	]`)
	p := &policy.Policy{ID: "p1", Name: "test", Rules: rules}

	d, err := e.Evaluate(context.Background(), p, policy.EvalContext{CostUSD: 50})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if d.RuleID != "r1" {
		t.Errorf("expected first rule match (r1), got %q", d.RuleID)
	}
}

func TestEngine_ToolNameInList(t *testing.T) {
	e := NewEngine()
	rules := json.RawMessage(`[
		{"id":"r1","kind":"permission","expression":"tool_name in [\"Bash\",\"Write\"]","action":"approval_required","priority":0}
	]`)
	p := &policy.Policy{ID: "p1", Name: "test", Rules: rules}

	cases := []struct {
		tool string
		want policy.Action
	}{
		{"Bash", policy.ActionApprovalRequired},
		{"Read", policy.ActionAllow},
	}
	for _, c := range cases {
		d, err := e.Evaluate(context.Background(), p, policy.EvalContext{ToolName: c.tool})
		if err != nil {
			t.Fatalf("evaluate tool=%q: %v", c.tool, err)
		}
		if d.Action != c.want {
			t.Errorf("tool=%q: action=%q, want %q", c.tool, d.Action, c.want)
		}
	}
}

func TestEngine_LogicalAnd(t *testing.T) {
	e := NewEngine()
	rules := json.RawMessage(`[
		{"id":"r1","kind":"cost","expression":"cost_usd > 0.5 && tokens_in > 500","action":"deny","priority":0}
	]`)
	p := &policy.Policy{ID: "p1", Name: "test", Rules: rules}

	cases := []struct {
		cost   float64
		tokens int64
		want   policy.Action
	}{
		{0.1, 100, policy.ActionAllow}, // both fail
		{1.0, 100, policy.ActionAllow}, // only cost matches
		{0.1, 600, policy.ActionAllow}, // only tokens match
		{1.0, 600, policy.ActionDeny},  // both match
	}
	for _, c := range cases {
		d, err := e.Evaluate(context.Background(), p, policy.EvalContext{CostUSD: c.cost, TokensIn: c.tokens})
		if err != nil {
			t.Fatalf("evaluate (%v, %v): %v", c.cost, c.tokens, err)
		}
		if d.Action != c.want {
			t.Errorf("(cost=%v, tokens=%v): action=%q, want %q", c.cost, c.tokens, d.Action, c.want)
		}
	}
}

func TestEngine_InvalidExpression(t *testing.T) {
	e := NewEngine()
	rules := json.RawMessage(`[
		{"id":"r1","kind":"cost","expression":"nope > 0","action":"deny","priority":0}
	]`)
	p := &policy.Policy{ID: "p1", Name: "test", Rules: rules}

	_, err := e.Evaluate(context.Background(), p, policy.EvalContext{})
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}