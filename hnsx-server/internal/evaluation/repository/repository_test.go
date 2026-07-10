package repository

import (
	"testing"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/evaluation/model"
)

func TestInMemoryRepository_SetAndRun(t *testing.T) {
	r := NewInMemoryRepository()

	set := &model.EvalSet{
		ID:       "set-1",
		DomainID: "domain-1",
		SetID:    "customer-service-eval",
		Cases: []model.EvalCase{
			{ID: "c1", Name: "greeting", Input: map[string]any{"msg": "hi"}, Expect: map[string]any{"ok": true}},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := r.SaveSet(set); err != nil {
		t.Fatalf("save set: %v", err)
	}

	got, err := r.SetByID("set-1")
	if err != nil {
		t.Fatalf("get set: %v", err)
	}
	if got.SetID != "customer-service-eval" {
		t.Fatalf("expected set id customer-service-eval, got %s", got.SetID)
	}

	sets, total, err := r.ListSets(10, 0)
	if err != nil {
		t.Fatalf("list sets: %v", err)
	}
	if total != 1 || len(sets) != 1 {
		t.Fatalf("expected 1 set, got total=%d len=%d", total, len(sets))
	}

	domainSets, err := r.SetsByDomain("domain-1")
	if err != nil {
		t.Fatalf("sets by domain: %v", err)
	}
	if len(domainSets) != 1 {
		t.Fatalf("expected 1 domain set, got %d", len(domainSets))
	}

	run := &model.EvalRun{
		ID:         "run-1",
		EvalSetID:  "set-1",
		DomainID:   "domain-1",
		State:      "running",
		TotalCases: 1,
		CreatedAt:  time.Now().UTC(),
	}
	if err := r.SaveRun(run); err != nil {
		t.Fatalf("save run: %v", err)
	}

	gotRun, err := r.RunByID("run-1")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if gotRun.State != "running" {
		t.Fatalf("expected running, got %s", gotRun.State)
	}

	runs, err := r.RunsBySet("set-1")
	if err != nil {
		t.Fatalf("runs by set: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
}

func TestInMemoryRepository_SetNotFound(t *testing.T) {
	r := NewInMemoryRepository()
	if _, err := r.SetByID("missing"); err != model.ErrEvalSetNotFound {
		t.Fatalf("expected ErrEvalSetNotFound, got %v", err)
	}
}

func TestInMemoryRepository_RunNotFound(t *testing.T) {
	r := NewInMemoryRepository()
	if _, err := r.RunByID("missing"); err != model.ErrEvalRunNotFound {
		t.Fatalf("expected ErrEvalRunNotFound, got %v", err)
	}
}

func TestInMemoryRepository_SaveResults(t *testing.T) {
	r := NewInMemoryRepository()
	run := &model.EvalRun{ID: "run1", EvalSetID: "set1", DomainID: "d1", State: "running"}
	if err := r.SaveRun(run); err != nil {
		t.Fatalf("save run: %v", err)
	}

	results := []model.EvalResult{
		{CaseID: "c1", Score: 1, Passed: true},
		{CaseID: "c2", Score: 0, Passed: false},
	}
	if err := r.SaveResults("run1", results); err != nil {
		t.Fatalf("save results: %v", err)
	}

	got, err := r.RunByID("run1")
	if err != nil {
		t.Fatalf("run by id: %v", err)
	}
	if len(got.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got.Results))
	}

	if err := r.SaveResults("missing", results); err != model.ErrEvalRunNotFound {
		t.Fatalf("expected ErrEvalRunNotFound for unknown run, got %v", err)
	}
}
