package agentruntime

import (
	"encoding/json"

	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
)

// CodeBuddyBackend implements agentruntime.Backend for CodeBuddy CLI.
type CodeBuddyBackend struct {
	*StreamJSONBackend
}

func NewCodeBuddyBackend() *CodeBuddyBackend {
	b := &CodeBuddyBackend{
		StreamJSONBackend: NewStreamJSONBackend("codebuddy", "codebuddy"),
	}
	b.BuildArgs = b.buildArgs
	b.ExtractResult = b.extractResult
	return b
}

func (b *CodeBuddyBackend) buildArgs(prompt string, opts agentruntime.ExecOptions) []string {
	args := []string{
		"-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	args = append(args, "--yolo") // skip tool prompts
	args = append(args, opts.ExtraArgs...)
	return args
}

func (b *CodeBuddyBackend) extractResult(line []byte) *agentruntime.Result {
	var raw struct {
		Type       string  `json:"type"`
		IsError    bool    `json:"is_error"`
		DurationMs int64   `json:"duration_ms"`
		TotalCost  float64 `json:"total_cost_usd"`
		Result     string  `json:"result"`
		Usage      struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(line, &raw); err != nil || raw.Type != "result" {
		return nil
	}
	r := &agentruntime.Result{
		Backend:    "codebuddy",
		Summary:    raw.Result,
		TokensIn:   raw.Usage.InputTokens,
		TokensOut:  raw.Usage.OutputTokens,
		CostUSD:    raw.TotalCost,
		DurationMs: raw.DurationMs,
	}
	if raw.IsError {
		r.ErrorMessage = "codebuddy reported error: " + raw.Result
	}
	return r
}

var _ agentruntime.Backend = (*CodeBuddyBackend)(nil)