package policy

import (
	"errors"
	"testing"

	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

func TestEvaluateGuardrailsContainsBlock(t *testing.T) {
	engine := NewEngine(spec.PolicySpec{
		Guardrails: []spec.GuardrailSpec{
			{
				ID:      "no-secrets",
				Type:    "contains",
				On:      "agent_text",
				Action:  "block",
				Config:  "password",
				Message: "output must not contain secrets",
			},
		},
	})

	decision := engine.EvaluateGuardrails(GuardrailEvent{Kind: "agent_text", Text: "my password is 123"})
	if !decision.Matched {
		t.Fatal("expected guardrail to match")
	}
	if decision.Action != "block" {
		t.Fatalf("expected block, got %s", decision.Action)
	}

	clean := engine.EvaluateGuardrails(GuardrailEvent{Kind: "agent_text", Text: "hello world"})
	if clean.Matched {
		t.Fatal("expected no match for clean output")
	}
}

func TestEvaluateGuardrailsRegexLog(t *testing.T) {
	engine := NewEngine(spec.PolicySpec{
		Guardrails: []spec.GuardrailSpec{
			{
				ID:      "ssn-like",
				Type:    "regex",
				On:      "agent_text",
				Action:  "log",
				Config:  `\b\d{3}-\d{2}-\d{4}\b`,
				Message: "possible SSN detected",
			},
		},
	})

	decision := engine.EvaluateGuardrails(GuardrailEvent{Kind: "agent_text", Text: "my ssn is 123-45-6789"})
	if !decision.Matched {
		t.Fatal("expected regex guardrail to match")
	}
	if decision.Action != "log" {
		t.Fatalf("expected log, got %s", decision.Action)
	}
}

func TestEvaluateGuardrailsWrongKindIgnored(t *testing.T) {
	engine := NewEngine(spec.PolicySpec{
		Guardrails: []spec.GuardrailSpec{
			{
				ID:     "no-secrets",
				Type:   "contains",
				On:     "agent_text",
				Action: "block",
				Config: "password",
			},
		},
	})

	decision := engine.EvaluateGuardrails(GuardrailEvent{Kind: "tool_call", Text: "password"})
	if decision.Matched {
		t.Fatal("expected guardrail to be ignored for different event kind")
	}
}

func TestEvaluateGuardrailsDefaultAction(t *testing.T) {
	engine := NewEngine(spec.PolicySpec{
		Guardrails: []spec.GuardrailSpec{
			{
				ID:     "missing-action",
				Type:   "contains",
				Config: "x",
			},
		},
	})

	decision := engine.EvaluateGuardrails(GuardrailEvent{Text: "x"})
	if !decision.Matched || decision.Action != "log" {
		t.Fatalf("expected default log action, got %+v", decision)
	}
}

func TestErrGuardrailBlockedIsError(t *testing.T) {
	if !errors.Is(ErrGuardrailBlocked, ErrGuardrailBlocked) {
		t.Fatal("ErrGuardrailBlocked should be an error")
	}
}
