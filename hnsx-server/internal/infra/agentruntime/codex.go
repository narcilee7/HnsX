package agentruntime

import (
	"encoding/json"

	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
)

// CodexBackend implements agentruntime.Backend for OpenAI Codex CLI.
// Codex uses the same stream-json envelope as Claude, so the default
// decoder works. The args differ: --quiet for non-interactive, --json
// for the stream format.
type CodexBackend struct {
	*StreamJSONBackend
}

func NewCodexBackend() *CodexBackend {
	b := &CodexBackend{
		StreamJSONBackend: NewStreamJSONBackend("codex", "codex"),
	}
	b.BuildArgs = b.buildArgs
	b.ExtractResult = b.extractResult
	return b
}

func (b *CodexBackend) buildArgs(prompt string, opts agentruntime.ExecOptions) []string {
	args := []string{
		"-q", prompt,
		"--json",
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	// Codex sandbox: -s danger-full-access bypasses the network/sandbox
	// gate for R1. R3 policy engine replaces this with per-call decisions.
	args = append(args, "--sandbox", "danger-full-access")
	args = append(args, opts.ExtraArgs...)
	return args
}

func (b *CodexBackend) extractResult(line []byte) *agentruntime.Result {
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
		Backend:    "codex",
		Summary:    raw.Result,
		TokensIn:   raw.Usage.InputTokens,
		TokensOut:  raw.Usage.OutputTokens,
		CostUSD:    raw.TotalCost,
		DurationMs: raw.DurationMs,
	}
	if raw.IsError {
		r.ErrorMessage = "codex reported error: " + raw.Result
	}
	return r
}

var _ agentruntime.Backend = (*CodexBackend)(nil)