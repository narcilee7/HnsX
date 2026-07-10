package commands

import (
	"fmt"

	"github.com/hnsx-io/hnsx/server/internal/app"
	"github.com/hnsx-io/hnsx/server/internal/tenant"
	"github.com/hnsx-io/hnsx/server/pkg/runtime"
)

// MarkSessionRunning transitions a pending session to running.
func MarkSessionRunning(state *app.State, tenantID tenant.ID, id string) (*app.RegisteredSession, error) {
	if state == nil {
		return nil, fmt.Errorf("nil app state")
	}
	sess, ok := state.LookupSession(tenantID, id)
	if !ok {
		return nil, ErrSessionNotFound
	}
	if sess.State != "pending" {
		return nil, fmt.Errorf("session state is %q, expected pending", sess.State)
	}
	state.UpdateSessionState(tenantID, id, "running")
	return sess, nil
}

// MarkSessionCompleted stores the result and transitions to completed.
func MarkSessionCompleted(state *app.State, tenantID tenant.ID, id string, result *runtime.Result) (*app.RegisteredSession, error) {
	if state == nil {
		return nil, fmt.Errorf("nil app state")
	}
	sess, ok := state.LookupSession(tenantID, id)
	if !ok {
		return nil, ErrSessionNotFound
	}
	state.SetSessionResult(tenantID, id, result)
	state.UpdateSessionState(tenantID, id, "completed")
	return sess, nil
}

// MarkSessionFailed transitions a session to failed.
func MarkSessionFailed(state *app.State, tenantID tenant.ID, id string) (*app.RegisteredSession, error) {
	if state == nil {
		return nil, fmt.Errorf("nil app state")
	}
	if _, ok := state.LookupSession(tenantID, id); !ok {
		return nil, ErrSessionNotFound
	}
	state.UpdateSessionState(tenantID, id, "failed")
	sess, _ := state.LookupSession(tenantID, id)
	return sess, nil
}
