// Package runner executes an EvalSet case-by-case against a domain spec, scores
// each case, persists per-case results, and finalizes the run aggregates.
//
// Execution uses the synchronous session Executor (one session per case). Cases
// run with bounded concurrency; an optional budget guard cancels the remaining
// work once accrued cost exceeds the domain's policy budget.
package runner

import (
	"context"
	"sync"
	"time"

	evalmodel "github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	"github.com/hnsx-io/hnsx/server/internal/evaluation/scorer"
	evalservice "github.com/hnsx-io/hnsx/server/internal/evaluation/service"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// Executor runs a single domain session synchronously and returns its result.
// It is satisfied by *pkg/session.Executor.
type Executor interface {
	Execute(ctx context.Context, s *domain.DomainSpec, trigger map[string]any) (*runtime.Result, error)
}

// CostFunc returns the accrued cost (USD) for a finished session. Optional;
// a nil CostFunc contributes zero cost.
type CostFunc func(sessionID string) float64

// EvalRunner is the common surface implemented by the local Runner and the
// worker-pool WorkerPoolRunner.
type EvalRunner interface {
	Run(ctx context.Context, run *evalmodel.EvalRun, set *evalmodel.EvalSet, domainSpec *domain.DomainSpec, budgetUSD float64) error
}

var _ EvalRunner = (*Runner)(nil)
var _ EvalRunner = (*WorkerPoolRunner)(nil)

// Runner executes eval sets.
type Runner struct {
	exec        Executor
	svc         *evalservice.Service
	cost        CostFunc
	concurrency int
}

// Option configures a Runner.
type Option func(*Runner)

const defaultConcurrency = 4

// WithConcurrency bounds how many cases run in parallel (default 4).
func WithConcurrency(n int) Option {
	return func(r *Runner) {
		if n > 0 {
			r.concurrency = n
		}
	}
}

// WithCostFunc wires a per-session cost lookup for budget accounting.
func WithCostFunc(fn CostFunc) Option {
	return func(r *Runner) { r.cost = fn }
}

// New constructs a Runner.
func New(exec Executor, svc *evalservice.Service, opts ...Option) *Runner {
	r := &Runner{exec: exec, svc: svc, concurrency: defaultConcurrency}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Run executes every case in set against s, scores it, persists per-case
// results, and finalizes the run. budgetUSD <= 0 disables the budget guard.
// Blocks until all cases finish or the budget is exceeded.
func (r *Runner) Run(ctx context.Context, run *evalmodel.EvalRun, set *evalmodel.EvalSet, s *domain.DomainSpec, budgetUSD float64) error {
	start := time.Now()
	cases := set.Cases
	results := make([]evalmodel.EvalResult, len(cases))

	var (
		mu        sync.Mutex
		totalCost float64
		budgetHit bool
	)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, r.concurrency)
	var wg sync.WaitGroup
	for i := range cases {
		wg.Add(1)
		go func(idx int, ec evalmodel.EvalCase) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			res := r.runCase(ctx, s, ec)

			mu.Lock()
			results[idx] = res
			totalCost += res.CostUSD
			if budgetUSD > 0 && totalCost > budgetUSD && !budgetHit {
				budgetHit = true
				cancel() // short-circuit any in-flight / not-yet-started cases
			}
			mu.Unlock()
		}(i, cases[i])
	}
	wg.Wait()

	var passed int
	var scoreSum float64
	for _, res := range results {
		if res.Passed {
			passed++
		}
		scoreSum += res.Score
	}
	avg := 0.0
	if len(results) > 0 {
		avg = scoreSum / float64(len(results))
	}
	durationMs := time.Since(start).Milliseconds()

	if err := r.svc.RecordResults(run.ID, results); err != nil {
		return err
	}
	if budgetHit {
		return r.svc.FailRun(run.ID, avg, passed, len(results), durationMs, totalCost)
	}
	return r.svc.FinishRun(run.ID, avg, passed, len(results), durationMs, totalCost)
}

func (r *Runner) runCase(ctx context.Context, s *domain.DomainSpec, ec evalmodel.EvalCase) evalmodel.EvalResult {
	caseStart := time.Now()
	sessID := runtime.NewSessionID(s.ID)
	cctx := runtime.WithSessionID(ctx, sessID)

	res := evalmodel.EvalResult{
		CaseID:    ec.ID,
		SessionID: sessID,
		CreatedAt: time.Now().UTC(),
	}

	result, err := r.exec.Execute(cctx, s, ec.Input)
	if err != nil {
		res.Passed = false
		res.Details = map[string]any{"error": err.Error()}
		res.DurationMs = time.Since(caseStart).Milliseconds()
		return res
	}

	var actual map[string]any
	if result != nil {
		actual = result.Output
	}
	v := scorer.Score(ec.Scorer, ec.Expect, actual)
	res.Score = v.Score
	res.Passed = v.Passed
	res.Actual = actual
	res.Details = v.Details
	res.DurationMs = time.Since(caseStart).Milliseconds()
	if r.cost != nil {
		res.CostUSD = r.cost(sessID)
	}
	return res
}
