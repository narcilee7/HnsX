package runner

import (
	"context"
	"errors"
	"testing"

	evalmodel "github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	evalrepo "github.com/hnsx-io/hnsx/server/internal/evaluation/repository"
	evalservice "github.com/hnsx-io/hnsx/server/internal/evaluation/service"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

type stubExecutor struct {
	fn func(trigger map[string]any) (*domain.Result, error)
}

func (e stubExecutor) Execute(_ context.Context, _ *domain.DomainSpec, trigger map[string]any) (*domain.Result, error) {
	return e.fn(trigger)
}

func newRun(t *testing.T, svc *evalservice.Service, set *evalmodel.EvalSet) *evalmodel.EvalRun {
	t.Helper()
	if err := svc.CreateSet(set); err != nil {
		t.Fatalf("create set: %v", err)
	}
	run := &evalmodel.EvalRun{
		ID:         "run-" + set.ID,
		EvalSetID:  set.ID,
		DomainID:   set.DomainID,
		State:      "running",
		TotalCases: len(set.Cases),
	}
	if err := svc.CreateRun(run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	return run
}

func TestRunner_ScoresAndAggregates(t *testing.T) {
	svc := evalservice.NewService(evalrepo.NewInMemoryRepository())
	set := &evalmodel.EvalSet{
		ID:       "set1",
		DomainID: "d1",
		Cases: []evalmodel.EvalCase{
			{ID: "c1", Input: map[string]any{"produce": "yes"}, Expect: map[string]any{"answer": "yes"}, Scorer: evalmodel.Scorer{Type: "exact"}},
			{ID: "c2", Input: map[string]any{"produce": "no"}, Expect: map[string]any{"answer": "yes"}, Scorer: evalmodel.Scorer{Type: "exact"}},
		},
	}
	run := newRun(t, svc, set)

	exec := stubExecutor{fn: func(trigger map[string]any) (*domain.Result, error) {
		return &domain.Result{State: "completed", Output: map[string]any{"answer": trigger["produce"]}}, nil
	}}
	r := New(exec, svc, WithConcurrency(2))

	if err := r.Run(context.Background(), run, set, &domain.DomainSpec{ID: "d1"}, 0); err != nil {
		t.Fatalf("run: %v", err)
	}

	got, err := svc.GetRun(run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.State != "completed" {
		t.Fatalf("state = %q, want completed", got.State)
	}
	if got.PassedCases != 1 || got.TotalCases != 2 {
		t.Fatalf("passed=%d total=%d, want 1/2", got.PassedCases, got.TotalCases)
	}
	if got.Score < 0.49 || got.Score > 0.51 {
		t.Fatalf("score = %v, want ~0.5", got.Score)
	}
	if len(got.Results) != 2 {
		t.Fatalf("expected 2 per-case results, got %d", len(got.Results))
	}
}

func TestRunner_ExecuteErrorScoresZero(t *testing.T) {
	svc := evalservice.NewService(evalrepo.NewInMemoryRepository())
	set := &evalmodel.EvalSet{
		ID:       "set2",
		DomainID: "d1",
		Cases: []evalmodel.EvalCase{
			{ID: "c1", Expect: map[string]any{"answer": "yes"}},
		},
	}
	run := newRun(t, svc, set)

	exec := stubExecutor{fn: func(map[string]any) (*domain.Result, error) {
		return nil, errors.New("adapter blew up")
	}}
	r := New(exec, svc)

	if err := r.Run(context.Background(), run, set, &domain.DomainSpec{ID: "d1"}, 0); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, _ := svc.GetRun(run.ID)
	if got.PassedCases != 0 || got.Score != 0 {
		t.Fatalf("failed case should score 0, got passed=%d score=%v", got.PassedCases, got.Score)
	}
	if got.Results[0].Details["error"] == nil {
		t.Fatalf("expected error detail on failed case")
	}
}

func TestRunner_BudgetGuardFailsRun(t *testing.T) {
	svc := evalservice.NewService(evalrepo.NewInMemoryRepository())
	set := &evalmodel.EvalSet{
		ID:       "set3",
		DomainID: "d1",
		Cases: []evalmodel.EvalCase{
			{ID: "c1", Expect: map[string]any{}},
			{ID: "c2", Expect: map[string]any{}},
		},
	}
	run := newRun(t, svc, set)

	exec := stubExecutor{fn: func(map[string]any) (*domain.Result, error) {
		return &domain.Result{State: "completed", Output: map[string]any{}}, nil
	}}
	// Each case costs 1.0; budget of 0.5 trips after the first case.
	r := New(exec, svc, WithConcurrency(1), WithCostFunc(func(string) float64 { return 1.0 }))

	if err := r.Run(context.Background(), run, set, &domain.DomainSpec{ID: "d1"}, 0.5); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, _ := svc.GetRun(run.ID)
	if got.State != "failed" {
		t.Fatalf("state = %q, want failed (budget exceeded)", got.State)
	}
	if got.TotalCostUSD < 1.0 {
		t.Fatalf("expected accrued cost recorded, got %v", got.TotalCostUSD)
	}
}
