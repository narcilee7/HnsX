package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/cli"
	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/wire"
)

// EvalSet is a collection of EvalCases that exercises a DomainSpec's
// behaviour end-to-end. The runner spawns the agent for each case and
// scores the output against the expected substring match.
type EvalSet struct {
	ID          string
	DomainID    string
	Description string
	Cases       []EvalCase
}

// EvalCase is one input/expectation pair.
type EvalCase struct {
	ID         string
	Name       string
	Input      string
	Expect     string // substring the agent's output must contain to pass
	MaxCostUSD float64
}

// EvalResult is one case's outcome after RunEval.
type EvalResult struct {
	CaseID      string  `json:"case_id"`
	Pass        bool    `json:"pass"`
	Score       float64 `json:"score"` // 0..1 — fraction of expected tokens matched
	Actual      string  `json:"actual"`
	CostUSD     float64 `json:"cost_usd"`
	DurationMs  int64   `json:"duration_ms"`
	FailureReason string `json:"failure_reason,omitempty"`
}

// EvalReport is the aggregated result of running an EvalSet.
type EvalReport struct {
	EvalSetID   string       `json:"eval_set_id"`
	DomainID    string       `json:"domain_id"`
	StartedAt   time.Time    `json:"started_at"`
	FinishedAt  time.Time    `json:"finished_at"`
	TotalCostUSD float64     `json:"total_cost_usd"`
	Score       float64      `json:"score"`      // 0..1
	PassRate    float64      `json:"pass_rate"`  // 0..1
	Cases       []EvalResult `json:"cases"`

	// Baseline (optional) for regression detection.
	BaselineID  string  `json:"baseline_id,omitempty"`
	BaselineScore float64 `json:"baseline_score,omitempty"`
	Regressed   bool    `json:"regressed,omitempty"`
}

// EvalRunner is the W19 reference implementation. It runs an EvalSet
// against a real agent subprocess and produces an EvalReport.
type EvalRunner struct {
	Policy     Policy
	Invoker    Invoker
	Defaults   ExecutorDefaults
	MaxCases   int // safety: cap concurrent case executions
}

// NewEvalRunner constructs an EvalRunner with sensible defaults.
func NewEvalRunner(p Policy, invoker Invoker, defaults ExecutorDefaults) *EvalRunner {
	return &EvalRunner{
		Policy:   p,
		Invoker:  invoker,
		Defaults: defaults,
		MaxCases: 8,
	}
}

// Run executes every case in the EvalSet concurrently (capped at MaxCases)
// and returns an aggregated EvalReport. baseline is optional: when set,
// the runner compares the resulting Score against baseline.Score and
// reports a regression when the drop exceeds 0.05 (5 points).
func (r *EvalRunner) Run(ctx context.Context, set EvalSet, baseline *EvalReport) *EvalReport {
	rep := &EvalReport{
		EvalSetID: set.ID,
		DomainID:  set.DomainID,
		StartedAt: time.Now(),
		Cases:     make([]EvalResult, len(set.Cases)),
	}
	if baseline != nil {
		rep.BaselineID = baseline.EvalSetID
		rep.BaselineScore = baseline.Score
	}

	sem := make(chan struct{}, r.MaxCases)
	var wg sync.WaitGroup
	var totalScore, totalCost, passed float64

	for i, c := range set.Cases {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, caseSpec EvalCase) {
			defer wg.Done()
			defer func() { <-sem }()

			start := time.Now()
			res := r.runCase(ctx, caseSpec)
			res.DurationMs = time.Since(start).Milliseconds()
			rep.Cases[idx] = res

			totalScore += res.Score
			totalCost += res.CostUSD
			if res.Pass {
				passed++
			}
		}(i, c)
	}
	wg.Wait()

	rep.FinishedAt = time.Now()
	rep.TotalCostUSD = totalCost
	if len(set.Cases) > 0 {
		rep.Score = totalScore / float64(len(set.Cases))
		rep.PassRate = passed / float64(len(set.Cases))
	}
	if baseline != nil && rep.Score < baseline.Score-0.05 {
		rep.Regressed = true
	}
	return rep
}

// runCase spawns one agent invocation and scores its output.
func (r *EvalRunner) runCase(ctx context.Context, c EvalCase) EvalResult {
	var captured strings.Builder
	inv := cliInvocationFor(c, r.Defaults, &captured)

	runErr := r.Invoker(ctx, inv)
	res := EvalResult{
		CaseID: c.ID,
		Pass:   false,
		Actual: captured.String(),
		CostUSD: r.Defaults.EstimatedCostUSD,
	}
	if runErr != nil {
		res.FailureReason = runErr.Error()
		return res
	}

	res.Score = scoreMatch(captured.String(), c.Expect)
	res.Pass = res.Score >= 0.5
	if !res.Pass && res.FailureReason == "" {
		res.FailureReason = fmt.Sprintf("expected substring %q not found", c.Expect)
	}
	return res
}

// scoreMatch computes the fraction of expected tokens present in actual.
// Empty expectation scores 1.0; empty output with non-empty expectation
// scores 0.0.
func scoreMatch(actual, expect string) float64 {
	if expect == "" {
		return 1.0
	}
	tokens := strings.Fields(expect)
	if len(tokens) == 0 {
		return 1.0
	}
	hits := 0
	for _, t := range tokens {
		if strings.Contains(actual, t) {
			hits++
		}
	}
	return float64(hits) / float64(len(tokens))
}

// cliInvocationFor builds an Invocation that captures the agent's stdout
// into the provided strings.Builder. P0 captures only the text stream;
// later phases also capture tool_use / tool_result separately.
func cliInvocationFor(c EvalCase, defaults ExecutorDefaults, capture *strings.Builder) cli.Invocation {
	return cli.Invocation{
		Command: defaults.Command,
		Args:    defaults.Args,
		Env:     defaults.Env,
		WorkDir: defaults.WorkDir,
		Prompt:  c.Input,
		OnMessage: func(m wire.TaskMessage) {
			if m.Type == "text" {
				capture.WriteString(m.Content)
				capture.WriteString("\n")
			}
		},
	}
}
