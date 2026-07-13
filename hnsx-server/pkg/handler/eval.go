package handler

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/hnsx-io/hnsx/server/internal/app"
	evalmodel "github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	evalrunner "github.com/hnsx-io/hnsx/server/internal/evaluation/runner"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/handler/viewmodel"
)

// ---------------------------------------------------------------------------
// Inputs
// ---------------------------------------------------------------------------

type RunEvalInput struct {
	TenantID      tenant.ID
	SetID         string
	DomainID      string
	DomainVersion string
	Orchestration string
	BaselineRunID string
}

type GetEvalRunInput struct {
	TenantID tenant.ID
	RunID    string
}

type ListEvalRunsInput struct {
	TenantID tenant.ID
	SetID    string
	Limit    int
	Offset   int
}

// ---------------------------------------------------------------------------
// Outputs
// ---------------------------------------------------------------------------

type RunEvalOutput struct {
	Run  *viewmodel.EvalRunStarted
	Cost float64
}

type GetEvalRunOutput struct {
	Run *viewmodel.EvalRunDetail
}

type ListEvalRunsOutput struct {
	Runs viewmodel.EvalRunList
}

// ---------------------------------------------------------------------------
// Handler methods
// ---------------------------------------------------------------------------

// RunEval starts an evaluation run for the given set.
func (h *Handler) RunEval(ctx context.Context, in RunEvalInput) (*RunEvalOutput, error) {
	defer h.hook(ctx, "eval.run",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("set_id", in.SetID),
	)()

	if h.App == nil || h.App.EvalService == nil {
		return nil, fmt.Errorf("eval service unavailable")
	}
	if h.App.WorkerService == nil {
		return nil, fmt.Errorf("eval runner requires a worker pool")
	}

	set, err := h.App.EvalService.GetSet(in.SetID)
	if err != nil {
		return nil, err
	}

	domainID := in.DomainID
	if domainID == "" {
		domainID = set.DomainID
	}
	dm, err := h.App.DomainService.Get(in.TenantID, domainID)
	if err != nil {
		return nil, err
	}
	d := app.DomainFromModel(dm)

	run := &evalmodel.EvalRun{
		ID:            uuid.NewString(),
		EvalSetID:     set.ID,
		DomainID:      domainID,
		DomainVersion: in.DomainVersion,
		Orchestration: in.Orchestration,
		BaselineRunID: in.BaselineRunID,
		State:         "running",
		TotalCases:    len(set.Cases),
	}
	if run.DomainVersion == "" {
		run.DomainVersion = d.Version
	}
	if run.Orchestration == "" && d.Spec != nil {
		run.Orchestration = string(d.Spec.Harness.Session.Mode)
	}
	if err := h.App.EvalService.CreateRun(run); err != nil {
		return nil, err
	}

	budget := 0.0
	if d.Spec != nil {
		budget = d.Spec.Harness.Policy.Budget.MaxCostUSD
	}
	traceSvc := h.App.TraceService
	costFn := func(sessionID string) float64 {
		if traceSvc == nil {
			return 0
		}
		agg, err := traceSvc.Aggregate([]string{sessionID})
		if err != nil {
			return 0
		}
		return agg.TotalCostUSD
	}

	er := evalrunner.NewWorkerPoolRunner(h.SessionCommands, h.App.SessionService, h.App.EvalService, costFn)

	go func() {
		_ = er.Run(ctx, run, set, d.Spec, budget)
	}()

	return &RunEvalOutput{
		Run:  &viewmodel.EvalRunStarted{RunID: run.ID, State: run.State},
		Cost: budget,
	}, nil
}

// GetEvalRun returns a single eval run detail.
func (h *Handler) GetEvalRun(ctx context.Context, in GetEvalRunInput) (*GetEvalRunOutput, error) {
	defer h.hook(ctx, "eval.run.get",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("run_id", in.RunID),
	)()

	if h.App == nil || h.App.EvalService == nil {
		return nil, fmt.Errorf("eval service unavailable")
	}
	run, err := h.App.EvalService.GetRun(in.RunID)
	if err != nil {
		return nil, err
	}
	return &GetEvalRunOutput{Run: evalRunToDetail(run)}, nil
}

// ListEvalRuns returns all runs for an eval set.
func (h *Handler) ListEvalRuns(ctx context.Context, in ListEvalRunsInput) (*ListEvalRunsOutput, error) {
	defer h.hook(ctx, "eval.run.list",
		zap.String("tenant_id", string(in.TenantID)),
		zap.String("set_id", in.SetID),
	)()

	if h.App == nil || h.App.EvalService == nil {
		return nil, fmt.Errorf("eval service unavailable")
	}
	runs, err := h.App.EvalService.RunsBySet(in.SetID)
	if err != nil {
		return nil, err
	}
	out := make([]viewmodel.EvalRunItem, 0, len(runs))
	for _, run := range runs {
		out = append(out, evalRunToItem(&run))
	}
	limit := in.Limit
	if limit <= 0 {
		limit = len(out)
	}
	return &ListEvalRunsOutput{Runs: viewmodel.EvalRunList{
		Items:  out,
		Total:  len(out),
		Limit:  limit,
		Offset: in.Offset,
	}}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func evalRunToItem(run *evalmodel.EvalRun) viewmodel.EvalRunItem {
	if run == nil {
		return viewmodel.EvalRunItem{}
	}
	return viewmodel.EvalRunItem{
		ID:            run.ID,
		EvalSetID:     run.EvalSetID,
		DomainID:      run.DomainID,
		DomainVersion: run.DomainVersion,
		State:         run.State,
		Score:         run.Score,
		Total:         run.TotalCases,
		Passed:        run.PassedCases,
		TotalCostUSD:  run.TotalCostUSD,
		DurationMs:    run.DurationMs,
		BaselineRunID: run.BaselineRunID,
	}
}

func evalRunToDetail(run *evalmodel.EvalRun) *viewmodel.EvalRunDetail {
	if run == nil {
		return nil
	}
	cases := make([]viewmodel.EvalCaseResult, 0, len(run.Results))
	for _, r := range run.Results {
		cases = append(cases, viewmodel.EvalCaseResult{
			CaseID:    r.CaseID,
			SessionID: r.SessionID,
			Score:     r.Score,
			Passed:    r.Passed,
			Actual:    r.Actual,
			Details:   r.Details,
		})
	}
	return &viewmodel.EvalRunDetail{
		ID:            run.ID,
		EvalSetID:     run.EvalSetID,
		DomainID:      run.DomainID,
		DomainVersion: run.DomainVersion,
		State:         run.State,
		Score:         run.Score,
		Total:         run.TotalCases,
		Passed:        run.PassedCases,
		TotalCostUSD:  run.TotalCostUSD,
		DurationMs:    run.DurationMs,
		BaselineRunID: run.BaselineRunID,
		Cases:         cases,
	}
}

// IsEvalSetNotFound reports whether err is an eval-set-not-found error.
func IsEvalSetNotFound(err error) bool {
	return errors.Is(err, evalmodel.ErrEvalSetNotFound)
}

// IsEvalRunNotFound reports whether err is an eval-run-not-found error.
func IsEvalRunNotFound(err error) bool {
	return errors.Is(err, evalmodel.ErrEvalRunNotFound)
}
