package repository

import (
	"testing"

	"github.com/hnsx-io/hnsx/server/internal/policy/model"
)

func TestInMemoryRepository_SaveAndByDomain(t *testing.T) {
	r := NewInMemoryRepository()

	p := &model.Policy{
		DomainID: "domain-1",
		Budget: model.Budget{
			MaxCostUSD: 1.0,
			MaxTurns:   10,
			MaxTokens:  1000,
		},
		Permissions: model.Permissions{
			AllowFileWrite: true,
			AllowShell:     true,
		},
		Guardrails: []model.Guardrail{
			{ID: "g1", Type: "keyword", Action: "block"},
		},
	}

	if err := r.Save(p); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := r.ByDomain("domain-1")
	if err != nil {
		t.Fatalf("by domain: %v", err)
	}
	if got.DomainID != "domain-1" {
		t.Fatalf("expected domain-1, got %s", got.DomainID)
	}
	if got.Budget.MaxTurns != 10 {
		t.Fatalf("expected max turns 10, got %d", got.Budget.MaxTurns)
	}
	if len(got.Guardrails) != 1 {
		t.Fatalf("expected 1 guardrail, got %d", len(got.Guardrails))
	}
}

func TestInMemoryRepository_PolicyNotFound(t *testing.T) {
	r := NewInMemoryRepository()
	if _, err := r.ByDomain("missing"); err != model.ErrPolicyNotFound {
		t.Fatalf("expected ErrPolicyNotFound, got %v", err)
	}
}

func TestInMemoryRepository_SaveInvalid(t *testing.T) {
	r := NewInMemoryRepository()
	if err := r.Save(nil); err == nil {
		t.Fatal("expected error for nil policy")
	}
	if err := r.Save(&model.Policy{}); err == nil {
		t.Fatal("expected error for empty domain id")
	}
}
