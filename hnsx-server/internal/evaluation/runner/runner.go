// Package runner executes an EvalSet case-by-case against a domain spec, scores
// each case, persists per-case results, and finalizes the run aggregates.
//
// Execution uses the worker-pool session scheduler: each case becomes a
// session, workers pull from the queue, and the runner aggregates results.
package runner

import (
	"context"

	evalmodel "github.com/hnsx-io/hnsx/server/internal/evaluation/model"
	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// EvalRunner is the common surface implemented by runner implementations.
type EvalRunner interface {
	Run(ctx context.Context, run *evalmodel.EvalRun, set *evalmodel.EvalSet, domainSpec *domain.DomainSpec, budgetUSD float64) error
}

// CostFunc returns the accrued cost (USD) for a finished session. Optional;
// a nil CostFunc contributes zero cost.
type CostFunc func(sessionID string) float64

var _ EvalRunner = (*WorkerPoolRunner)(nil)
