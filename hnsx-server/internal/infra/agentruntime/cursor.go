package agentruntime

import (
	"encoding/json"

	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
)

// CursorBackend implements agentruntime.Backend by spawning the
// `cursor-agent` CLI in non-interactive mode with stream-json output.
// Wire format mirrors Claude Code: newline-delimited JSON with a
// "type" field.
type CursorBackend struct {
	*StreamJSONBackend
}

// NewCursorBackend wires a StreamJSONBackend with cursor-specific args
// and result extraction.
func NewCursorBackend() *CursorBackend {
	b := &CursorBackend{
		StreamJSONBackend: NewStreamJSONBackend("cursor", "cursor-agent"),
	}
	b.BuildArgs = b.buildArgs
	b.ExtractResult = b.extractResult
	return b
}

func (b *CursorBackend) buildArgs(prompt string, opts agentruntime.ExecOptions) []string {
	args := []string{
		"-p", prompt,
		"--output-format", "stream-json",
		"--verbose",
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", opts.SystemPrompt)
	}
	// R1: skip per-tool permission prompts (R3 policy engine replaces).
	args = append(args, "--trust-all-tools")
	args = append(args, opts.ExtraArgs...)
	return args
}

// extractResult handles cursor-agent's "result" event shape (subset of
// claude's: duration_ms, total_cost_usd, usage).
func (b *CursorBackend) extractResult(line []byte) *agentruntime.Result {
	var raw struct {
		Type        string  `json:"type"`
		Subtype     string  `json:"subtype"`
		IsError     bool    `json:"is_error"`
		DurationMs  int64   `json:"duration_ms"`
		TotalCost   float64 `json:"total_cost_usd"`
		Result      string  `json:"result"`
		Usage       struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(line, &raw); err != nil || raw.Type != "result" {
		return nil
	}
	r := &agentruntime.Result{
		Backend:    "cursor",
		Summary:    raw.Result,
		TokensIn:   raw.Usage.InputTokens,
		TokensOut:  raw.Usage.OutputTokens,
		CostUSD:    raw.TotalCost,
		DurationMs: raw.DurationMs,
	}
	if raw.IsError {
		r.ErrorMessage = "cursor-agent reported error: " + raw.Result
	}
	return r
}

var _ agentruntime.Backend = (*CursorBackend)(nil)