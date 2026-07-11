package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/app/commands"
	evalmodel "github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	"github.com/hnsx-io/hnsx/server/internal/evaluation/scorer"
	evalservice "github.com/hnsx-io/hnsx/server/internal/evaluation/service"
	"github.com/hnsx-io/hnsx/server/internal/session/model"
	sessionservice "github.com/hnsx-io/hnsx/server/internal/session/service"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

var pollInterval = 500 * time.Millisecond

// WorkerPoolRunner executes an EvalSet by dispatching each case as a normal
// session through SessionCommands. Cases run on the worker pool (or the local
// executor fallback) concurrently; the runner polls the session service until
// every case reaches a terminal state, then scores and finalizes the run.
type WorkerPoolRunner struct {
	sessionCmds *commands.SessionCommands
	sessionSvc  *sessionservice.Service
	evalSvc     *evalservice.Service
	cost        CostFunc
}

// NewWorkerPoolRunner constructs a runner backed by session commands.
func NewWorkerPoolRunner(
	sessionCmds *commands.SessionCommands,
	sessionSvc *sessionservice.Service,
	evalSvc *evalservice.Service,
	cost CostFunc,
) *WorkerPoolRunner {
	return &WorkerPoolRunner{
		sessionCmds: sessionCmds,
		sessionSvc:  sessionSvc,
		evalSvc:     evalSvc,
		cost:        cost,
	}
}

// Run executes every case in set against domainSpec.
// budgetUSD <= 0 disables the budget guard. The runner returns only after all
// cases have reached a terminal state or ctx is cancelled. The tenant ID is
// read from ctx via tenant.FromContext.
func (r *WorkerPoolRunner) Run(ctx context.Context, run *evalmodel.EvalRun, set *evalmodel.EvalSet, domainSpec *spec.DomainSpec, budgetUSD float64) error {
	if r.sessionCmds == nil {
		return fmt.Errorf("worker pool runner: session commands not configured")
	}
	if r.sessionSvc == nil {
		return fmt.Errorf("worker pool runner: session service not configured")
	}

	tenantID := tenant.FromContext(ctx)
	domain := &app.RegisteredDomain{
		ID:      run.DomainID,
		Version: run.DomainVersion,
		Spec:    domainSpec,
	}

	start := time.Now()
	sessions := make([]*app.RegisteredSession, 0, len(set.Cases))
	caseBySession := map[string]evalmodel.EvalCase{}

	for _, c := range set.Cases {
		sess, err := r.sessionCmds.Start(ctx, tenantID, domain, c.Input)
		if err != nil {
			_ = r.finalize(run.ID, sessions, caseBySession, tenantID, start, true)
			return fmt.Errorf("dispatch eval case %s: %w", c.ID, err)
		}
		sessions = append(sessions, sess)
		caseBySession[sess.ID] = c
	}

	if err := r.waitTerminal(ctx, sessions, tenantID); err != nil {
		_ = r.finalize(run.ID, sessions, caseBySession, tenantID, start, true)
		return err
	}

	return r.finalize(run.ID, sessions, caseBySession, tenantID, start, false)
}

func (r *WorkerPoolRunner) waitTerminal(ctx context.Context, sessions []*app.RegisteredSession, tenantID tenant.ID) error {
	pending := make(map[string]bool, len(sessions))
	for _, s := range sessions {
		pending[s.ID] = true
	}
	for len(pending) > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
		for sid := range pending {
			sess, err := r.sessionSvc.Get(tenantID, sid)
			if err != nil {
				continue
			}
			if sess.IsTerminal() {
				delete(pending, sid)
			}
		}
	}
	return nil
}

func (r *WorkerPoolRunner) finalize(runID string, sessions []*app.RegisteredSession, caseBySession map[string]evalmodel.EvalCase, tenantID tenant.ID, start time.Time, errored bool) error {
	results := make([]evalmodel.EvalResult, 0, len(sessions))
	var (
		passed    int
		scoreSum  float64
		totalCost float64
	)

	for _, sess := range sessions {
		c, ok := caseBySession[sess.ID]
		if !ok {
			continue
		}
		stateSess, err := r.sessionSvc.Get(tenantID, sess.ID)
		res := evalmodel.EvalResult{
			CaseID:    c.ID,
			SessionID: sess.ID,
			CreatedAt: time.Now().UTC(),
		}
		if err != nil {
			res.Passed = false
			res.Details = map[string]any{"error": err.Error()}
		} else {
			duration := stateSess.Duration()
			res.DurationMs = duration.Milliseconds()
			if stateSess.State == model.StateCompleted && stateSess.Result != nil {
				v := scorer.Score(c.Scorer, c.Expect, stateSess.Result.Output)
				res.Score = v.Score
				res.Passed = v.Passed
				res.Actual = stateSess.Result.Output
				res.Details = v.Details
			} else {
				res.Passed = false
				res.Details = map[string]any{"state": string(stateSess.State)}
			}
			if r.cost != nil {
				res.CostUSD = r.cost(sess.ID)
			}
		}
		if res.Passed {
			passed++
		}
		scoreSum += res.Score
		totalCost += res.CostUSD
		results = append(results, res)
	}

	avg := 0.0
	if len(results) > 0 {
		avg = scoreSum / float64(len(results))
	}

	durationMs := time.Since(start).Milliseconds()
	if err := r.evalSvc.RecordResults(runID, results); err != nil {
		return err
	}
	if errored {
		return r.evalSvc.FailRun(runID, avg, passed, len(results), durationMs, totalCost)
	}
	return r.evalSvc.FinishRun(runID, avg, passed, len(results), durationMs, totalCost)
}
