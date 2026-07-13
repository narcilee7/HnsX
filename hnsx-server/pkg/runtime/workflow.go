package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/hnsx-io/hnsx/server/pkg/domain"
)

func (r *Runner) runWorkflow(
	ctx context.Context,
	specArg *domain.DomainSpec,
	trigger map[string]any,
	res *Result,
) error {
	wf := specArg.Harness.Session.Workflow
	if wf == nil {
		return errors.New("workflow definition missing")
	}

	byID := make(map[string]domain.StepSpec, len(wf.Steps))
	for _, s := range wf.Steps {
		byID[s.ID] = s
	}
	if _, ok := byID[wf.Entry]; !ok {
		return fmt.Errorf("workflow.entry %q not found", wf.Entry)
	}

	step := byID[wf.Entry]
	visited := map[string]bool{}

	vars := mergeVars(trigger, nil)
	if wf.Variables != nil {
		if m, ok := wf.Variables.(map[string]any); ok {
			vars = mergeVars(m, vars)
		}
	}

	for step.ID != "" {
		if visited[step.ID] {
			return fmt.Errorf("workflow cycle detected at step %q", step.ID)
		}
		visited[step.ID] = true

		agent, ok := specArg.Harness.Agents[step.Agent]
		if !ok {
			return fmt.Errorf("step %q references unknown agent %q", step.ID, step.Agent)
		}

		prompt, err := resolvePrompt(specArg, agent)
		if err != nil {
			return err
		}

		input := buildStepInput(step.Input, vars)

		r.publish(Observation{
			Kind:      "step_start",
			SessionID: res.SessionID,
			DomainID:  res.DomainID,
			StepID:    step.ID,
			AgentID:   step.Agent,
			Timestamp: time.Now().UTC(),
		})

		out, err := r.adapter.Invoke(ctx, agent, prompt, input)
		if err != nil {
			r.publish(Observation{
				Kind:      "error",
				SessionID: res.SessionID,
				DomainID:  res.DomainID,
				StepID:    step.ID,
				AgentID:   step.Agent,
				Payload:   map[string]any{"code": "ADAPTER_ERROR", "message": err.Error()},
				Timestamp: time.Now().UTC(),
			})
			return fmt.Errorf("step %s: %w", step.ID, err)
		}

		if step.Output != "" {
			vars[step.Output] = out
			res.Output[step.Output] = out
		}

		r.publish(Observation{
			Kind:      "step_end",
			SessionID: res.SessionID,
			DomainID:  res.DomainID,
			StepID:    step.ID,
			AgentID:   step.Agent,
			Payload:   map[string]any{"output": out},
			Timestamp: time.Now().UTC(),
		})

		if step.Next == "" {
			return nil
		}
		next, ok := byID[step.Next]
		if !ok {
			return fmt.Errorf("step %q points to unknown next %q", step.ID, step.Next)
		}
		step = next
	}
	return nil
}

func mergeVars(base map[string]any, override map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		if _, ok := out[k]; !ok {
			out[k] = v
		}
	}
	return out
}

func buildStepInput(raw any, vars map[string]any) map[string]any {
	out := map[string]any{}
	if raw != nil {
		if m, ok := raw.(map[string]any); ok {
			for k, v := range m {
				out[k] = walkInterpolate(v, vars)
			}
		}
	}
	for k, v := range vars {
		if _, set := out[k]; !set {
			out[k] = v
		}
	}
	return out
}

func walkInterpolate(v any, vars map[string]any) any {
	switch val := v.(type) {
	case string:
		return expandString(val, vars)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, item := range val {
			out[k] = walkInterpolate(item, vars)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = walkInterpolate(item, vars)
		}
		return out
	default:
		return val
	}
}

func encodeJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
