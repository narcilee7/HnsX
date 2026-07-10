package repository

import (
	"testing"

	"github.com/hnsx-io/hnsx/server/internal/policy/model"
)

func TestInMemoryRepository_SaveAndByID(t *testing.T) {
	r := NewInMemoryRepository()

	p := &model.Policy{
		ID:          "policy-billing",
		Name:        "Billing safety net",
		Description: "Block shell + guard refund cost",
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

	got, err := r.ByID("policy-billing")
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if got.ID != "policy-billing" || got.Name != "Billing safety net" {
		t.Fatalf("id/name wrong: %+v", got)
	}
	if got.Budget.MaxTurns != 10 {
		t.Fatalf("expected max turns 10, got %d", got.Budget.MaxTurns)
	}
	if len(got.Guardrails) != 1 {
		t.Fatalf("expected 1 guardrail, got %d", len(got.Guardrails))
	}
}

func TestInMemoryRepository_ByDomain_BindOneToOne(t *testing.T) {
	r := NewInMemoryRepository()
	for _, id := range []string{"p1", "p2"} {
		if err := r.Save(&model.Policy{ID: id, Name: id}); err != nil {
			t.Fatalf("save %s: %v", id, err)
		}
	}
	if err := r.BindDomain("p1", "billing"); err != nil {
		t.Fatalf("bind p1->billing: %v", err)
	}
	if err := r.BindDomain("p2", "billing"); err != nil {
		t.Fatalf("bind p2->billing: %v", err)
	}
	got, err := r.ByDomain("billing")
	if err != nil {
		t.Fatalf("by domain: %v", err)
	}
	// The most recent bind wins; p2 must hold the slot.
	if got.ID != "p2" {
		t.Fatalf("expected p2 to be bound, got %s", got.ID)
	}
	// And p1 must no longer be bound to anything.
	if p1, _ := r.ByID("p1"); p1.BoundDomain != "" {
		t.Fatalf("p1 still bound: %+v", p1)
	}
}

func TestInMemoryRepository_Unbind(t *testing.T) {
	r := NewInMemoryRepository()
	if err := r.Save(&model.Policy{ID: "p1", Name: "p1", BoundDomain: "billing"}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := r.BindDomain("p1", ""); err != nil {
		t.Fatalf("unbind: %v", err)
	}
	if _, err := r.ByDomain("billing"); err != model.ErrPolicyNotFound {
		t.Fatalf("expected ErrPolicyNotFound after unbind, got %v", err)
	}
}

func TestInMemoryRepository_List(t *testing.T) {
	r := NewInMemoryRepository()
	if err := r.Save(&model.Policy{ID: "a", Name: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Save(&model.Policy{ID: "b", Name: "B", BoundDomain: "billing"}); err != nil {
		t.Fatal(err)
	}
	list, err := r.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	if list[0].ID != "a" || list[1].ID != "b" {
		t.Fatalf("order wrong: %+v", list)
	}
	if list[1].BoundDomain != "billing" {
		t.Fatalf("binding missing: %+v", list[1])
	}
}

func TestInMemoryRepository_Delete(t *testing.T) {
	r := NewInMemoryRepository()
	if err := r.Save(&model.Policy{ID: "p1", BoundDomain: "billing"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete("p1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := r.ByID("p1"); err != model.ErrPolicyNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
	if _, err := r.ByDomain("billing"); err != model.ErrPolicyNotFound {
		t.Fatalf("expected domain binding cleared, got %v", err)
	}
	if err := r.Delete("missing"); err != model.ErrPolicyNotFound {
		t.Fatalf("expected not found on missing delete, got %v", err)
	}
}

func TestInMemoryRepository_PolicyNotFound(t *testing.T) {
	r := NewInMemoryRepository()
	if _, err := r.ByDomain("missing"); err != model.ErrPolicyNotFound {
		t.Fatalf("expected ErrPolicyNotFound, got %v", err)
	}
	if _, err := r.ByID("missing"); err != model.ErrPolicyNotFound {
		t.Fatalf("expected ErrPolicyNotFound, got %v", err)
	}
}

func TestInMemoryRepository_SaveInvalid(t *testing.T) {
	r := NewInMemoryRepository()
	if err := r.Save(nil); err == nil {
		t.Fatal("expected error for nil policy")
	}
	if err := r.Save(&model.Policy{}); err != model.ErrInvalidPolicyID {
		t.Fatalf("expected ErrInvalidPolicyID, got %v", err)
	}
}
