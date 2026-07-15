package engine

import (
	"context"
	"fmt"

	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/cli"
	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/wire"
)

// WireClient is the subset of *wire.Daemon the executor depends on.
// Defining it here makes the executor testable without a live server.
type WireClient interface {
	ReportMessage(ctx context.Context, runtimeID, taskID string, msg wire.TaskMessage) error
	ReportProgress(ctx context.Context, runtimeID, taskID, summary string, step, total int) error
	ReportComplete(ctx context.Context, runtimeID, taskID, prURL, output string) error
	ReportFail(ctx context.Context, runtimeID, taskID, errMsg string) error
}

// Executor ties together the policy engine, the agent subprocess layer,
// and the wire client that streams observations back to the server.
//
// P0 (W6) implements the synchronous claim → spawn → stream → complete
// lifecycle. Pause / Resume / cancel-from-server arrive in W7.
type Executor struct {
	Policy   Policy
	Defaults ExecutorDefaults
	Wire     WireClient
	Invoker  Invoker
}

// ExecutorDefaults carries runtime defaults applied when a task doesn't
// pin them explicitly.
type ExecutorDefaults struct {
	// Command is the CLI binary to spawn (e.g. "claude"). Empty means
	// "look up by AgentID from the runtime registry".
	Command string
	// Args is the default argv (stream-json flags etc.).
	Args []string
	// Env is the default environment merged with per-task overrides.
	Env []string
	// WorkDir is the working directory for the subprocess.
	WorkDir string
	// EstimatedCostUSD is the projected cost when the agent spec doesn't
	// provide one. Used by the policy gate.
	EstimatedCostUSD float64
}

// Invoker is the minimal interface the executor needs to spawn a
// subprocess. cli.Run satisfies it; tests can inject fakes.
type Invoker func(ctx context.Context, inv cli.Invocation) error

// NewExecutor constructs an Executor with sane defaults.
func NewExecutor(w WireClient, p Policy, defaults ExecutorDefaults) *Executor {
	return &Executor{
		Policy:   p,
		Defaults: defaults,
		Wire:     w,
		Invoker:  cli.Run,
	}
}

// Execute runs one claimed task end-to-end: policy gate → spawn →
// stream observations → mark complete / failed. The caller is responsible
// for actually receiving the task from ClaimTask and for running this in
// a worker goroutine.
func (e *Executor) Execute(ctx context.Context, task *wire.Task) error {
	if task == nil {
		return fmt.Errorf("executor: nil task")
	}
	runtimeID := task.RuntimeID

	// 1. Policy gate.
	action := Action{
		DomainID:         task.AgentID,
		AgentID:          task.AgentID,
		Kind:             "session.start",
		EstimatedCostUSD: e.Defaults.EstimatedCostUSD,
		Resource:         "",
	}
	dec := e.Policy.Check(ctx, action)
	if !dec.Allow {
		_ = e.Wire.ReportMessage(ctx, runtimeID, task.ID, wire.TaskMessage{
			Type:    "error",
			Content: "policy denied: " + dec.Reason,
		})
		return e.Wire.ReportFail(ctx, runtimeID, task.ID, "policy denied: "+dec.Reason)
	}
	if dec.RequireApproval {
		_ = e.Wire.ReportMessage(ctx, runtimeID, task.ID, wire.TaskMessage{
			Type:    "text",
			Content: "approval_required: " + dec.Reason,
		})
	}

	// 2. Wire the cancel channel so server-pushed cancel signals abort
	//    the subprocess promptly. W18 adds WS-based cancel; P0 only
	//    honors ctx cancellation (Ctrl-C / SIGTERM).
	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 3. Build the brief from the trigger.
	prompt, _ := task.Trigger["issue_description"].(string)
	if prompt == "" {
		prompt, _ = task.Trigger["issue_title"].(string)
	}
	if prompt == "" {
		prompt = "Run the assigned task for agent " + task.AgentID
	}

	// 4. Spawn the agent subprocess and stream output back via the wire.
	inv := cli.Invocation{
		Command: e.Defaults.Command,
		Args:    e.Defaults.Args,
		Env:     e.Defaults.Env,
		WorkDir: e.Defaults.WorkDir,
		Prompt:  prompt,
		OnMessage: func(msg wire.TaskMessage) {
			_ = e.Wire.ReportMessage(cancelCtx, runtimeID, task.ID, msg)
		},
		OnProgress: func(summary string, step, total int) {
			_ = e.Wire.ReportProgress(cancelCtx, runtimeID, task.ID, summary, step, total)
		},
	}

	runErr := e.Invoker(cancelCtx, inv)

	// 5. Distinguish a real subprocess failure from a graceful cancel.
	if runErr != nil && cancelCtx.Err() != nil {
		// Server-pushed cancel or shutdown — emit cancel observation,
		// leave the session in a paused/canceled state for the next phase.
		_ = e.Wire.ReportMessage(ctx, runtimeID, task.ID, wire.TaskMessage{
			Type:    "error",
			Content: "task canceled: " + cancelCtx.Err().Error(),
		})
		return e.Wire.ReportFail(ctx, runtimeID, task.ID, "canceled")
	}

	// 6. Mark terminal.
	if runErr != nil {
		return e.Wire.ReportFail(ctx, runtimeID, task.ID, runErr.Error())
	}
	return e.Wire.ReportComplete(ctx, runtimeID, task.ID, "", prompt)
}
