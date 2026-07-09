package commands

import (
	"context"
	"fmt"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/pkg/adapter"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
	"github.com/hnsx-io/hnsx/server/pkg/spec"
)

// RunLocalSession executes a DomainSpec locally using the given adapter.
// This is the shared backend for `hnsx run` and server-side local execution.
func RunLocalSession(ctx context.Context, s *spec.DomainSpec, trigger map[string]any, a runtime.Adapter) (*runtime.Result, error) {
	runner := runtime.NewRunner(a)
	return runner.Run(ctx, s, trigger)
}

// PickAdapter returns a built-in adapter by kind.
func PickAdapter(kind string) (runtime.Adapter, error) {
	switch kind {
	case "noop", "":
		return adapter.NewNoopAdapter(), nil
	case "echo":
		return adapter.NewEchoAdapter(), nil
	default:
		return nil, fmt.Errorf("unknown adapter: %s (built-in: noop, echo)", kind)
	}
}

// MarkSessionRunning transitions a pending session to running.
func MarkSessionRunning(state *app.State, id string) (*app.RegisteredSession, error) {
	if state == nil {
		return nil, fmt.Errorf("nil app state")
	}
	sess, ok := state.LookupSession(id)
	if !ok {
		return nil, ErrSessionNotFound
	}
	if sess.State != "pending" {
		return nil, fmt.Errorf("session state is %q, expected pending", sess.State)
	}
	state.UpdateSessionState(id, "running")
	return sess, nil
}

// MarkSessionCompleted stores the result and transitions to completed.
func MarkSessionCompleted(state *app.State, id string, result *runtime.Result) (*app.RegisteredSession, error) {
	if state == nil {
		return nil, fmt.Errorf("nil app state")
	}
	sess, ok := state.LookupSession(id)
	if !ok {
		return nil, ErrSessionNotFound
	}
	state.SetSessionResult(id, result)
	state.UpdateSessionState(id, "completed")
	return sess, nil
}

// MarkSessionFailed transitions a session to failed.
func MarkSessionFailed(state *app.State, id string) (*app.RegisteredSession, error) {
	if state == nil {
		return nil, fmt.Errorf("nil app state")
	}
	if _, ok := state.LookupSession(id); !ok {
		return nil, ErrSessionNotFound
	}
	state.UpdateSessionState(id, "failed")
	sess, _ := state.LookupSession(id)
	return sess, nil
}
