// Package agentruntime hosts concrete Backend implementations for every
// agent CLI hnsxd can spawn. The package-level contract is the
// domain.agentruntime.Backend / Session / Registry ports; this file
// (and claude.go, codex.go, ... which land in R2) are the implementations.
//
// R1.8 lands only the Claude Backend so we can get a "spawn claude and
// stream output" smoke working without DB / HTTP / CLI surface.
// R2 ports the remaining 24 backends from multica_fork/pkg/agent.
package agentruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
)

// stderrTailBytes is how much of the subprocess stderr we keep in memory
// for the Result.ErrorMessage field when the CLI exits non-zero. Bigger
// tails give better diagnostics for V8/Bun crashes, but cost RAM.
const stderrTailBytes = 8 * 1024

// claudeRunner carries the resolved config + logger that a ClaudeBackend
// needs to spawn the `claude` CLI. Wiring happens in app.New.
type claudeRunner struct {
	execPath string
	logger   *slog.Logger
}

// NewClaudeRunner constructs a runner from the resolved binary path and
// the application's logger. execPath defaults to "claude" if empty,
// matching the PATH lookup convention used by Multica's agent/claude.go.
func NewClaudeRunner(execPath string, logger *slog.Logger) *claudeRunner {
	if execPath == "" {
		execPath = "claude"
	}
	return &claudeRunner{execPath: execPath, logger: logger}
}

// ClaudeBackend implements domain.agentruntime.Backend by spawning the
// Anthropic Claude Code CLI in non-interactive print mode with stream-json
// output. The output line format is the documented `claude --output-format
// stream-json --verbose` protocol.
type ClaudeBackend struct {
	r *claudeRunner
}

// NewClaudeBackend wires a runner into a Backend port.
func NewClaudeBackend(r *claudeRunner) *ClaudeBackend {
	return &ClaudeBackend{r: r}
}

// Name returns "claude" — the registry key.
func (b *ClaudeBackend) Name() string { return "claude" }

// Execute spawns the claude CLI and returns a Session that streams
// messages via its Messages() channel and resolves to a Result via Result().
//
// Args built:
//   -p <prompt>                            (non-interactive, print mode)
//   --output-format stream-json            (newline-delimited JSON events)
//   --verbose                              (required for stream-json)
//   --model <model>                        (if opts.Model != "")
//   --dangerously-skip-permissions         (R1: skip tool prompts so smoke is reproducible;
//                                           R3 policy engine replaces this with real approval)
//   --append-system-prompt <prompt>        (if opts.SystemPrompt != "")
//
// The subprocess is killed when ctx is cancelled or when Session.Cancel is
// called. The caller's goroutine reads stdout via a bufio.Scanner; the
// scanner and the subprocess share stdout, so a slow reader will block the
// CLI which can then block on stdin if we wrote the prompt there. We avoid
// that by passing the prompt via the -p flag (not stdin) for R1.
func (b *ClaudeBackend) Execute(ctx context.Context, prompt string, opts agentruntime.ExecOptions) (agentruntime.Session, error) {
	if b.r == nil {
		return nil, errors.New("claude backend: nil runner")
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("claude backend: empty prompt")
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)

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
	// R1 smoke is reproducible only if we bypass the per-tool permission
	// prompts. R3 (policy engine) replaces this with real per-call decisions.
	args = append(args, "--dangerously-skip-permissions")
	args = append(args, opts.ExtraArgs...)

	cmd := exec.CommandContext(runCtx, b.r.execPath, args...)
	cmd.WaitDelay = 5 * time.Second

	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}
	env := append([]string{}, osEnviron()...)
	for k, v := range opts.ExtraEnv {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("claude: stdout pipe: %w", err)
	}
	stderr := newStderrTail(b.r.logger, stderrTailBytes)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("claude: start: %w", err)
	}
	b.r.logger.Info("claude: spawned",
		"exec", b.r.execPath,
		"args", args,
		"cwd", opts.Cwd,
		"model", opts.Model,
		"pid", cmd.Process.Pid,
	)

	sess := newClaudeSession(runCtx, cancel, cmd, stdout, stderr, b.r.logger)
	go sess.run()
	return sess, nil
}

// claudeSession implements domain.agentruntime.Session against a running
// `claude` subprocess.
type claudeSession struct {
	ctx    context.Context
	cancel context.CancelFunc
	cmd    *exec.Cmd
	stdout io.ReadCloser
	stderr *stderrTail

	msgs   chan agentruntime.Message
	result chan *agentruntime.Result

	cancelOnce sync.Once
	logger     *slog.Logger
}

func newClaudeSession(
	ctx context.Context,
	cancel context.CancelFunc,
	cmd *exec.Cmd,
	stdout io.ReadCloser,
	stderr *stderrTail,
	logger *slog.Logger,
) *claudeSession {
	return &claudeSession{
		ctx:    ctx,
		cancel: cancel,
		cmd:    cmd,
		stdout: stdout,
		stderr: stderr,
		msgs:   make(chan agentruntime.Message, 64),
		result: make(chan *agentruntime.Result, 1),
		logger: logger,
	}
}

// Messages yields decoded Message values until the subprocess exits.
// The channel is closed by run() after the final result is published.
func (s *claudeSession) Messages() <-chan agentruntime.Message { return s.msgs }

// Result blocks until the subprocess exits and returns the final result.
// Returns ctx.Err() if the caller cancelled before the subprocess ended.
func (s *claudeSession) Result() (*agentruntime.Result, error) {
	res, ok := <-s.result
	if !ok {
		return nil, errors.New("claude session: result channel closed without result")
	}
	if res.ErrorMessage != "" {
		return res, errors.New(res.ErrorMessage)
	}
	return res, nil
}

// Cancel signals the subprocess to terminate. Safe to call concurrently.
// Idempotent: subsequent calls are no-ops.
func (s *claudeSession) Cancel(ctx context.Context) error {
	s.cancelOnce.Do(func() {
		s.cancel()
	})
	return nil
}

// run is the per-session loop: scan stdout, decode messages, publish them,
// wait for exit, publish a Result. Closes msgs and result when done.
func (s *claudeSession) run() {
	defer close(s.msgs)
	defer close(s.result)

	var seq int64
	var lastResult agentruntime.Result
	lastResult.Backend = "claude"
	scanner := bufio.NewScanner(s.stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // up to 1 MiB per line

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		msg, ok := decodeClaudeStreamLine(line, seq)
		if !ok {
			continue // unknown event type — skip silently
		}
		seq++
		if msg.Kind == agentruntime.MsgAssistant ||
			msg.Kind == agentruntime.MsgToolUse ||
			msg.Kind == agentruntime.MsgToolResult ||
			msg.Kind == agentruntime.MsgProgress ||
			msg.Kind == agentruntime.MsgSystem {
			select {
			case s.msgs <- msg:
			case <-s.ctx.Done():
				return
			}
		}
		if r := extractClaudeResult(line); r != nil {
			lastResult = *r
			lastResult.Backend = "claude"
		}
	}

	if err := scanner.Err(); err != nil {
		lastResult.ErrorMessage = "stdout read: " + err.Error()
		s.logger.Warn("claude: stdout scanner error", "err", err)
	}

	waitErr := s.cmd.Wait()
	if waitErr != nil {
		if lastResult.ErrorMessage == "" {
			lastResult.ErrorMessage = waitErr.Error() + ": " + s.stderr.Tail()
		} else {
			lastResult.ErrorMessage += " | " + waitErr.Error() + ": " + s.stderr.Tail()
		}
		s.logger.Warn("claude: subprocess exited with error",
			"err", waitErr,
			"stderr_tail", s.stderr.Tail(),
		)
	}
	if lastResult.DurationMs == 0 {
		// Fallback duration when no result event was observed.
		lastResult.DurationMs = 0
	}
	s.result <- &lastResult
}

// decodeClaudeStreamLine parses one JSON line of the claude stream-json
// protocol. Unknown event types are skipped (returns ok=false) so future
// Claude versions can introduce new event kinds without breaking us.
func decodeClaudeStreamLine(line []byte, seq int64) (agentruntime.Message, bool) {
	var raw struct {
		Type    string          `json:"type"`
		Subtype string          `json:"subtype"`
		Message json.RawMessage `json:"message"`
		Content json.RawMessage `json:"content"`
		Name    string          `json:"name"`
		Input   json.RawMessage `json:"input"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(line, &raw); err != nil {
		return agentruntime.Message{}, false
	}

	var kind agentruntime.MessageKind
	switch raw.Type {
	case "assistant":
		kind = agentruntime.MsgAssistant
	case "user":
		// user events include tool_result; classify more precisely below.
		kind = agentruntime.MsgToolResult
	case "tool_use":
		kind = agentruntime.MsgToolUse
	case "tool_result":
		kind = agentruntime.MsgToolResult
	case "progress":
		kind = agentruntime.MsgProgress
	case "system":
		kind = agentruntime.MsgSystem
	case "result":
		// result is handled by extractClaudeResult, not the message stream.
		return agentruntime.Message{}, false
	case "error":
		kind = agentruntime.MsgError
	default:
		return agentruntime.Message{}, false
	}

	payload := map[string]any{
		"type":    raw.Type,
		"subtype": raw.Subtype,
	}
	if len(raw.Message) > 0 {
		payload["message"] = json.RawMessage(raw.Message)
	}
	if len(raw.Content) > 0 {
		payload["content"] = json.RawMessage(raw.Content)
	}
	if raw.Name != "" {
		payload["name"] = raw.Name
	}
	if len(raw.Input) > 0 {
		payload["input"] = json.RawMessage(raw.Input)
	}
	if len(raw.Error) > 0 {
		payload["error"] = json.RawMessage(raw.Error)
	}
	payloadJSON, _ := json.Marshal(payload)

	preview := string(line)
	if len(preview) > 200 {
		preview = preview[:200] + "…"
	}

	return agentruntime.Message{
		Kind:     kind,
		Payload:  payloadJSON,
		Raw:      preview,
		Sequence: seq,
	}, true
}

// extractClaudeResult pulls the final cost / duration / token fields from
// a result event. Returns nil for non-result lines.
func extractClaudeResult(line []byte) *agentruntime.Result {
	var raw struct {
		Type        string  `json:"type"`
		Subtype     string  `json:"subtype"`
		IsError     bool    `json:"is_error"`
		DurationMs  int64   `json:"duration_ms"`
		DurationAPI int64   `json:"duration_api_ms"`
		NumTurns    int     `json:"num_turns"`
		Result      string  `json:"result"`
		TotalCost   float64 `json:"total_cost_usd"`
		Usage       struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(line, &raw); err != nil || raw.Type != "result" {
		return nil
	}
	r := &agentruntime.Result{
		ExitCode:   0,
		Summary:    raw.Result,
		TokensIn:   raw.Usage.InputTokens,
		TokensOut:  raw.Usage.OutputTokens,
		CostUSD:    raw.TotalCost,
		DurationMs: raw.DurationMs,
	}
	if raw.IsError {
		r.ErrorMessage = "claude reported error: " + raw.Result
	}
	return r
}