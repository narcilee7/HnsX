package agentruntime

import (
	"encoding/json"

	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
)

// PiBackend implements agentruntime.Backend for Pi CLI.
type PiBackend struct {
	*StreamJSONBackend
}

func NewPiBackend() *PiBackend {
	b := &PiBackend{
		StreamJSONBackend: NewStreamJSONBackend("pi", "pi"),
	}
	b.BuildArgs = b.buildArgs
	b.ExtractResult = b.extractResult
	return b
}

func (b *PiBackend) buildArgs(prompt string, opts agentruntime.ExecOptions) []string {
	args := []string{
		"-p", prompt,
		"--output-format", "stream-json",
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	args = append(args, "--trust-all-tools")
	args = append(args, opts.ExtraArgs...)
	return args
}

func (b *PiBackend) extractResult(line []byte) *agentruntime.Result {
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
		Backend:    "pi",
		Summary:    raw.Result,
		TokensIn:   raw.Usage.InputTokens,
		TokensOut:  raw.Usage.OutputTokens,
		CostUSD:    raw.TotalCost,
		DurationMs: raw.DurationMs,
	}
	if raw.IsError {
		r.ErrorMessage = "pi reported error: " + raw.Result
	}
	return r
}

var _ agentruntime.Backend = (*PiBackend)(nil)