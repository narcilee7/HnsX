// Package runtime drives Harness sessions from a DomainSpec v2.
//
// Phase 1 supports three orchestration shapes:
//
//   - single / single-task / multi-turn: invoke the named primary agent once.
//   - workflow: walk the static DAG (see workflow.go).
//   - supervisor / hierarchical / autonomous: rejected with a clear
//     "not yet implemented" error so users get fast feedback.
//
// Observations emitted by the runner flow through:
//
//   - Per-runner Observer hook (Runner.WithHook) — used by Executor to
//     fan out to a broadcaster and telemetry sinks.
//   - Per-step values stored in Result.Output for callers to read.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

// Adapter is the contract for invoking an external Agent.
// Implementations live in pkg/adapter and MUST be safe for concurrent use.
type Adapter interface {
	// Name returns the adapter kind (e.g. "noop", "echo", "anthropic").
	Name() string
	// Invoke calls the underlying agent and returns its text reply.
	Invoke(ctx context.Context, agent domain.AgentSpec, prompt string, input map[string]any) (string, error)
}

// ObservationHook is invoked by Runner for every observation it produces.
// Implementations MUST NOT block; if you need blocking work, fan out via a
// channel or goroutine.
type ObservationHook func(Observation)

// Runner is the entrypoint for executing a DomainSpec.
type Runner struct {
	adapter Adapter
	mu      sync.Mutex
	hook    ObservationHook
}

// NewRunner constructs a Runner bound to a single adapter.
// The same runner is safe to use from multiple goroutines.
func NewRunner(adapter Adapter) *Runner {
	return &Runner{adapter: adapter}
}

// WithHook wires a callback that receives every observation produced by the
// runner. Pass nil to disable.
func (r *Runner) WithHook(h ObservationHook) *Runner {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hook = h
	return r
}

func (r *Runner) publish(obs Observation) {
	r.mu.Lock()
	h := r.hook
	r.mu.Unlock()
	if h == nil {
		return
	}
	if obs.Timestamp.IsZero() {
		obs.Timestamp = time.Now().UTC()
	}
	h(obs)
}

// Result is the structured outcome of a single session run.
type Result struct {
	SessionID  string                  `json:"session_id"`
	DomainID   string                  `json:"domain_id"`
	State      string                  `json:"state"`
	Mode       domain.HarnessSessionMode `json:"mode"`
	Output     map[string]any          `json:"output"`
	StartedAt  time.Time               `json:"started_at,omitempty"`
	FinishedAt time.Time               `json:"finished_at,omitempty"`
}

// ErrSupervisorNotImplemented is returned for orchestration modes that
// require dynamic routing (supervisor / hierarchical / autonomous). They are
// scheduled for Phase 2 in server/pkg/session/supervisor.go.
var ErrSupervisorNotImplemented = errors.New(
	"session mode is not yet implemented in this build")

// Run executes a DomainSpec end-to-end and returns the result. Trigger is forwarded
// to the agent as its initial input.
func (r *Runner) Run(ctx context.Context, ds *domain.DomainSpec, trigger map[string]any) (*Result, error) {
	if ds == nil {
		return nil, errors.New("nil domain spec")
	}
	if r.adapter == nil {
		return nil, errors.New("nil adapter")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	sessID := SessionIDFromContext(ctx)
	if sessID == "" {
		sessID = NewSessionID(ds.ID)
	}

	res := &Result{
		SessionID: sessID,
		DomainID:  ds.ID,
		State:     "running",
		Mode:      ds.Harness.Session.Mode,
		Output:    map[string]any{},
		StartedAt: time.Now().UTC(),
	}

	r.publish(Observation{
		Kind:      "session_start",
		SessionID: res.SessionID,
		DomainID:  res.DomainID,
		Payload:   map[string]any{"trigger": trigger},
	})

	var runErr error
	switch ds.Harness.Session.Mode {
	case domain.Single, domain.SingleTask, domain.MultiTurn:
		runErr = r.runSingle(ctx, ds, trigger, res)
	case domain.Workflow:
		runErr = r.runWorkflow(ctx, ds, trigger, res)
	case domain.Supervisor, domain.Hierarchical, domain.Autonomous:
		runErr = fmt.Errorf("%w (mode=%s, build=phase1)",
			ErrSupervisorNotImplemented, ds.Harness.Session.Mode)
	default:
		runErr = fmt.Errorf("unknown session mode: %q", ds.Harness.Session.Mode)
	}

	res.FinishedAt = time.Now().UTC()

	if runErr != nil {
		res.State = "failed"
		r.publish(Observation{
			Kind:      "session_end",
			SessionID: res.SessionID,
			DomainID:  res.DomainID,
			Payload:   map[string]any{"state": "failed", "error": runErr.Error()},
		})
		return res, runErr
	}

	res.State = "completed"
	r.publish(Observation{
		Kind:      "session_end",
		SessionID: res.SessionID,
		DomainID:  res.DomainID,
		Payload:   map[string]any{"state": "completed"},
	})
	return res, nil
}

// runSingle executes the named primary agent once.
func (r *Runner) runSingle(ctx context.Context, ds *domain.DomainSpec, trigger map[string]any, res *Result) error {
	agentName := ds.Harness.Session.Agent
	if agentName == "" {
		for name := range ds.Harness.Agents {
			agentName = name
			break
		}
	}
	if agentName == "" {
		return errors.New("no agent selected for single mode")
	}
	agent, ok := ds.Harness.Agents[agentName]
	if !ok {
		return fmt.Errorf("primary agent %q not found", agentName)
	}

	prompt, err := resolvePrompt(ds, agent)
	if err != nil {
		return err
	}

	r.publish(Observation{
		Kind:      "agent_invoke",
		SessionID: res.SessionID,
		DomainID:  res.DomainID,
		AgentID:   agentName,
		Payload:   map[string]any{"adapter": r.adapter.Name()},
	})

	out, err := r.adapter.Invoke(ctx, agent, prompt, trigger)
	if err != nil {
		r.publish(Observation{
			Kind:      "error",
			SessionID: res.SessionID,
			DomainID:  res.DomainID,
			AgentID:   agentName,
			Payload:   map[string]any{"code": "ADAPTER_ERROR", "message": err.Error()},
		})
		return err
	}

	res.Output["response"] = out
	res.Output["agent"] = agentName
	r.publish(Observation{
		Kind:      "agent_text",
		SessionID: res.SessionID,
		DomainID:  res.DomainID,
		AgentID:   agentName,
		Payload:   map[string]any{"content": out},
	})
	return nil
}

// resolvePrompt returns the prompt template configured for the agent.
// If the agent references a named prompt via system_prompt, we look it up in
// harness.prompts; otherwise the system_prompt string is treated verbatim.
func resolvePrompt(ds *domain.DomainSpec, agent domain.AgentSpec) (string, error) {
	if agent.SystemPrompt == "" {
		return "", nil
	}
	if p, ok := ds.Harness.Prompts[agent.SystemPrompt]; ok {
		return p.Template, nil
	}
	return agent.SystemPrompt, nil
}
