package agentruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
)

// StreamJSONBackend is the shared subprocess + JSON-line scanner that
// ~80% of agent CLIs use (claude, codex, cursor, copilot, codebuddy,
// opencode, qoder, traecli, antigravity, hermes, kimi, kiro, ...).
//
// Concrete backends supply 4 hooks (Name / Executable / BuildArgs /
// DecodeLine) and inherit everything else: subprocess management,
// stdin/stdout deadlock prevention, stderr tail capture, message
// channel publication, result extraction.
//
// Backend-specific tests override DecodeLine for backend-specific event
// shapes; the boilerplate around it stays shared.
type StreamJSONBackend struct {
	name string // registry key

	// Executable overrides the lookup path. Empty → use DefaultExecutable.
	Executable string

	// DefaultExecutable is the PATH lookup name.
	DefaultExecutable string

	// Logger is the slog logger; never nil after NewStreamJSONBackend.
	Logger *slog.Logger

	// BuildArgs constructs the CLI invocation. MUST be set.
	BuildArgs func(prompt string, opts agentruntime.ExecOptions) []string

	// DecodeLine parses one JSON line. If it returns ok=false the line
	// is skipped. If nil, the default decoder is used.
	DecodeLine func(line []byte, seq int64) (agentruntime.Message, bool)

	// ExtractResult pulls the final Result from a result line. If nil,
	// no result is extracted (caller falls back to exit-code-only).
	ExtractResult func(line []byte) *agentruntime.Result
}

// NewStreamJSONBackend returns a StreamJSONBackend with sensible defaults
// filled in. BuildArgs and DecodeLine remain required.
func NewStreamJSONBackend(name, defaultExecutable string) *StreamJSONBackend {
	return &StreamJSONBackend{
		name:             name,
		DefaultExecutable: defaultExecutable,
		DecodeLine:       defaultDecodeStreamLine,
	}
}

// Name implements agentruntime.Backend.
func (b *StreamJSONBackend) Name() string { return b.name }

// Executable returns the resolved executable path.
func (b *StreamJSONBackend) exec() string {
	if b.Executable != "" {
		return b.Executable
	}
	return b.DefaultExecutable
}

// Execute implements agentruntime.Backend.
func (b *StreamJSONBackend) Execute(ctx context.Context, prompt string, opts agentruntime.ExecOptions) (agentruntime.Session, error) {
	if b.BuildArgs == nil {
		return nil, fmt.Errorf("%s backend: BuildArgs not set", b.name)
	}
	if strings.TrimSpace(prompt) == "" {
		return nil, fmt.Errorf("%s backend: empty prompt", b.name)
	}
	if b.Logger == nil {
		b.Logger = slog.Default()
	}

	execPath := b.exec()
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("%s executable not found at %q: %w", b.name, execPath, err)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)

	args := b.BuildArgs(prompt, opts)
	cmd := exec.CommandContext(runCtx, execPath, args...)
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
		return nil, fmt.Errorf("%s: stdout pipe: %w", b.name, err)
	}
	stderr := newStderrTail(b.Logger, stderrTailBytes)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("%s: start: %w", b.name, err)
	}
	b.Logger.Info("agent: spawned",
		"backend", b.name,
		"exec", execPath,
		"args", args,
		"cwd", opts.Cwd,
		"pid", cmd.Process.Pid,
	)

	sess := newGenericSession(runCtx, cancel, cmd, stdout, stderr, b.Logger, b.ExtractResult)
	go sess.run()
	return sess, nil
}

// defaultDecodeStreamLine is the default DecodeLine used by all backends
// unless they override. It understands the common "type" field and the
// Claude/Codex/Cursor shared event shapes (assistant / user / tool_use
// / tool_result / progress / error / system).
func defaultDecodeStreamLine(line []byte, seq int64) (agentruntime.Message, bool) {
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
		kind = agentruntime.MsgToolResult
	case "tool_use":
		kind = agentruntime.MsgToolUse
	case "tool_result":
		kind = agentruntime.MsgToolResult
	case "progress":
		kind = agentruntime.MsgProgress
	case "system":
		kind = agentruntime.MsgSystem
	case "error":
		kind = agentruntime.MsgError
	default:
		return agentruntime.Message{}, false
	}

	payload := map[string]any{"type": raw.Type, "subtype": raw.Subtype}
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

// genericSession is the shared run loop used by all stream-json backends.
type genericSession struct {
	ctx      context.Context
	cancel   context.CancelFunc
	cmd      *exec.Cmd
	stdout   interface{ Read(p []byte) (int, error) }
	stderr   *stderrTail

	msgs   chan agentruntime.Message
	result chan *agentruntime.Result

	cancelOnce sync.Once
	logger     *slog.Logger
	extract    func(line []byte) *agentruntime.Result
}

func newGenericSession(
	ctx context.Context,
	cancel context.CancelFunc,
	cmd *exec.Cmd,
	stdout interface {
		Read(p []byte) (int, error)
	},
	stderr *stderrTail,
	logger *slog.Logger,
	extract func(line []byte) *agentruntime.Result,
) *genericSession {
	return &genericSession{
		ctx:     ctx,
		cancel:  cancel,
		cmd:     cmd,
		stdout:  stdout,
		stderr:  stderr,
		msgs:    make(chan agentruntime.Message, 64),
		result:  make(chan *agentruntime.Result, 1),
		logger:  logger,
		extract: extract,
	}
}

// Messages yields decoded Message values until the subprocess exits.
func (s *genericSession) Messages() <-chan agentruntime.Message { return s.msgs }

// Result blocks until the subprocess exits and returns the final result.
func (s *genericSession) Result() (*agentruntime.Result, error) {
	res, ok := <-s.result
	if !ok {
		return nil, fmt.Errorf("session: result channel closed without result")
	}
	if res != nil && res.ErrorMessage != "" {
		return res, fmt.Errorf("%s", res.ErrorMessage)
	}
	if res == nil {
		return &agentruntime.Result{}, nil
	}
	return res, nil
}

// Cancel signals the subprocess to terminate.
func (s *genericSession) Cancel(ctx context.Context) error {
	s.cancelOnce.Do(func() {
		s.cancel()
	})
	return nil
}

// run is the per-session loop.
func (s *genericSession) run() {
	defer close(s.msgs)
	defer close(s.result)

	// Adapter for stdout via bufio.Scanner — Scanner needs an io.Reader.
	r, ok := s.stdout.(interface {
		Read(p []byte) (n int, err error)
	})
	if !ok {
		s.result <- &agentruntime.Result{ErrorMessage: "stdout does not implement io.Reader"}
		return
	}

	var seq int64
	var lastResult agentruntime.Result
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if s.extract != nil {
			if r := s.extract(line); r != nil {
				lastResult = *r
			}
		}
		if msg, ok := defaultDecodeStreamLine(line, seq); ok {
			seq++
			select {
			case s.msgs <- msg:
			case <-s.ctx.Done():
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		lastResult.ErrorMessage = "stdout read: " + err.Error()
		s.logger.Warn("agent: stdout scanner error", "err", err)
	}

	waitErr := s.cmd.Wait()
	if waitErr != nil {
		if lastResult.ErrorMessage == "" {
			lastResult.ErrorMessage = waitErr.Error() + ": " + s.stderr.Tail()
		} else {
			lastResult.ErrorMessage += " | " + waitErr.Error() + ": " + s.stderr.Tail()
		}
	}
	s.result <- &lastResult
}

// _ ensures genericSession implements agentruntime.Session.
var _ agentruntime.Session = (*genericSession)(nil)