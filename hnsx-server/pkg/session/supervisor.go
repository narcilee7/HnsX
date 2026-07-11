package session

import (
	"context"
	"errors"

	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// ErrSupervisorNotImplemented is returned by Supervisor.Run because the
// supervisor / hierarchical orchestration mode is gated behind a future PR.
var ErrSupervisorNotImplemented = errors.New(
	"supervisor orchestration mode is not yet implemented; use mode=workflow in this build",
)

// Supervisor is the placeholder for the supervisor orchestration strategy.
// It exists so the runtime can be type-routed from `Runner.Run` even when
// supervisor support is incomplete — the goal is fast, predictable feedback
// ("not yet implemented" rather than silent fallback).
type Supervisor struct{}

// NewSupervisor constructs an empty Supervisor.
func NewSupervisor() *Supervisor { return &Supervisor{} }

// Run returns ErrSupervisorNotImplemented. Future PRs will replace this with
// the transition-rule engine that drives dynamic dispatch between specialists
// (see docs/know-how/我们如何编排Agent并集成Harness.md §3.2).
func (s *Supervisor) Run(_ context.Context, _ *spec.DomainSpec, _ map[string]any) (*runtime.Result, error) {
	return nil, ErrSupervisorNotImplemented
}
