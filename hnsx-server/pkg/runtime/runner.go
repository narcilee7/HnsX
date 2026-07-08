// Package runtime executes Harness sessions.
package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hnsx-io/hnsx/go/pkg/core"
)

// Runner executes Harness sessions.
type Runner struct {
	adapter Adapter
}

// Adapter is the interface to external agents.
type Adapter interface {
	Invoke(ctx context.Context, agent core.Agent, prompt string, input map[string]interface{}) (string, error)
}

// NewRunner creates a new Runner with the given adapter.
func NewRunner(adapter Adapter) *Runner {
	return &Runner{adapter: adapter}
}

// SessionResult is the outcome of a session.
type SessionResult struct {
	SessionID string                 `json:"session_id"`
	DomainID  string                 `json:"domain_id"`
	State     string                 `json:"state"`
	Output    map[string]interface{} `json:"output"`
	StartedAt time.Time              `json:"started_at"`
	CompletedAt time.Time            `json:"completed_at"`
}

// Run executes a single session for a domain spec and trigger payload.
func (r *Runner) Run(ctx context.Context, spec *core.DomainSpec, payload map[string]interface{}) (*SessionResult, error) {
	result := &SessionResult{
		SessionID: fmt.Sprintf("%s-%d", spec.ID, time.Now().Unix()),
		DomainID:  spec.ID,
		State:     "running",
		StartedAt: time.Now(),
		Output:    make(map[string]interface{}),
	}

	switch spec.Harness.Session.Mode {
	case "single-task":
		if err := r.runSingleTask(ctx, spec, payload, result); err != nil {
			result.State = "failed"
			return result, err
		}
	case "workflow":
		if err := r.runWorkflow(ctx, spec, payload, result); err != nil {
			result.State = "failed"
			return result, err
		}
	default:
		result.State = "failed"
		return result, fmt.Errorf("unsupported session mode: %s", spec.Harness.Session.Mode)
	}

	result.State = "completed"
	result.CompletedAt = time.Now()
	return result, nil
}

func (r *Runner) runSingleTask(ctx context.Context, spec *core.DomainSpec, payload map[string]interface{}, result *SessionResult) error {
	agent := spec.Harness.Agents[0]
	prompt := buildPrompt(spec, agent)
	output, err := r.adapter.Invoke(ctx, agent, prompt, payload)
	if err != nil {
		return err
	}
	result.Output["response"] = output
	return nil
}

func (r *Runner) runWorkflow(ctx context.Context, spec *core.DomainSpec, payload map[string]interface{}, result *SessionResult) error {
	wf := spec.Harness.Session.Workflow
	agentMap := make(map[string]core.Agent)
	for _, a := range spec.Harness.Agents {
		agentMap[a.ID] = a
	}

	stepMap := make(map[string]core.Step)
	for _, s := range wf.Steps {
		stepMap[s.ID] = s
	}

	variables := make(map[string]interface{})
	for k, v := range payload {
		variables[k] = v
	}

	current := wf.Entry
	visited := make(map[string]bool)
	for current != "" {
		if visited[current] {
			return fmt.Errorf("workflow cycle detected at step %s", current)
		}
		visited[current] = true

		step, ok := stepMap[current]
		if !ok {
			return fmt.Errorf("workflow step %s not found", current)
		}

		agent, ok := agentMap[step.AgentRef]
		if !ok {
			return fmt.Errorf("agent %s not found", step.AgentRef)
		}

		prompt := buildPrompt(spec, agent)
		input := make(map[string]interface{})
		for k, v := range step.Input {
			input[k] = interpolate(v, variables)
		}
		for k, v := range variables {
			if _, ok := input[k]; !ok {
				input[k] = v
			}
		}

		output, err := r.adapter.Invoke(ctx, agent, prompt, input)
		if err != nil {
			return fmt.Errorf("step %s: %w", step.ID, err)
		}

		if step.Output != "" {
			variables[step.Output] = output
			result.Output[step.Output] = output
		}

		if len(step.Next) > 0 {
			current = step.Next[0]
		} else {
			current = ""
		}
	}

	return nil
}

func buildPrompt(spec *core.DomainSpec, agent core.Agent) string {
	if agent.Prompt.ID != "" {
		for _, p := range spec.Harness.Prompts {
			if p.ID == agent.Prompt.ID {
				return p.Template
			}
		}
	}
	return ""
}

func interpolate(template string, variables map[string]interface{}) string {
	// Simple ${var} interpolation.
	for k, v := range variables {
		switch val := v.(type) {
		case string:
			template = replaceAll(template, "${"+k+"}", val)
		default:
			b, _ := json.Marshal(val)
			template = replaceAll(template, "${"+k+"}", string(b))
		}
	}
	return template
}

func replaceAll(s, old, new string) string {
	for {
		i := 0
		for i+len(old) <= len(s) {
			if s[i:i+len(old)] == old {
				s = s[:i] + new + s[i+len(old):]
				i += len(new)
			} else {
				i++
			}
		}
		if i == len(s) {
			break
		}
	}
	return s
}
