package engine

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/cli"
	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/wire"
)

// TestExecutor_CancelViaContext verifies that canceling the parent ctx
// surfaces as a "canceled" failure (not a real subprocess error).
func TestExecutor_CancelViaContext(t *testing.T) {
	var failMsg string
	d := &fakeDaemon{
		onFail: func(taskID, msg string) error {
			failMsg = msg
			return nil
		},
	}
	started := make(chan struct{})
	invoker := func(ctx context.Context, inv cli.Invocation) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}

	e := &Executor{
		Policy:   &FlatPolicy{},
		Wire:     d,
		Invoker:  invoker,
		Defaults: ExecutorDefaults{},
	}
	task := &wire.Task{ID: "task-cancel", AgentID: "x", RuntimeID: "rt-1"}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-started
		cancel()
	}()
	_ = e.Execute(ctx, task)
	if failMsg != "canceled" {
		t.Fatalf("expected failMsg=canceled; got %q", failMsg)
	}
}

// TestExecutor_NotCanceledOnSuccess verifies the cancel detection does NOT
// fire when the subprocess completes successfully.
func TestExecutor_NotCanceledOnSuccess(t *testing.T) {
	var completed int32
	d := &fakeDaemon{
		onComplete: func(_, _, _ string) error {
			atomic.AddInt32(&completed, 1)
			return nil
		},
	}
	e := &Executor{
		Policy:   &FlatPolicy{},
		Wire:     d,
		Invoker:  func(context.Context, cli.Invocation) error { return nil },
		Defaults: ExecutorDefaults{},
	}
	task := &wire.Task{ID: "task-ok", AgentID: "x", RuntimeID: "rt-1"}
	_ = e.Execute(context.Background(), task)
	if atomic.LoadInt32(&completed) != 1 {
		t.Fatalf("expected exactly one complete; got %d", completed)
	}
}

// TestExecutor_SubprocessError verifies a non-cancel error is reported as a
// regular failure (not "canceled").
func TestExecutor_SubprocessError(t *testing.T) {
	var failMsg string
	d := &fakeDaemon{
		onFail: func(_, msg string) error { failMsg = msg; return nil },
	}
	e := &Executor{
		Policy:   &FlatPolicy{},
		Wire:     d,
		Invoker:  func(context.Context, cli.Invocation) error { return errors.New("boom") },
		Defaults: ExecutorDefaults{},
	}
	task := &wire.Task{ID: "task-err", AgentID: "x", RuntimeID: "rt-1"}
	_ = e.Execute(context.Background(), task)
	if failMsg != "boom" {
		t.Fatalf("expected failMsg=boom; got %q", failMsg)
	}
}
