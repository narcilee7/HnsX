package agentruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
)

// ACPBackend implements agentruntime.Backend for CLIs that speak the
// Agent Client Protocol (JSON-RPC 2.0 over stdin/stdout).
//
// ACP is the protocol used by hermes-cli, kimi-cli, kiro-cli and others.
// The runtime contract is: send `initialize`, then `session/new`, then
// stream `session/prompt` notifications, listen for `session/update`
// notifications that carry per-event content blocks.
//
// The base here implements the wire-level plumbing (handshake,
// session, JSON-RPC framing, result resolution) and leaves
// per-backend quirks (specific initialize args, model handling,
// permission negotiation) to config hooks. hermes/kimi/kiro share
// the same wire and only differ in argv and initialize params.
type ACPBackend struct {
	cfg ACPConfig
}

// ACPConfig configures an ACP backend.
type ACPConfig struct {
	// Name is the registry key (e.g. "hermes", "kimi", "kiro").
	Name string
	// Executable is the binary name or absolute path.
	Executable string
	// ExtraArgs are prepended before the standard `acp` subcommand.
	ExtraArgs []string
	// InitParams customizes the JSON-RPC `initialize` call.
	// Default: {"protocolVersion":"1","clientInfo":{"name":"hnsxd","version":"0.1.0"}}
	InitParams map[string]any
	// Timeout is the per-call default; zero → 5m.
	Timeout time.Duration
}

// NewACPBackend constructs a backend with the given config.
func NewACPBackend(cfg ACPConfig) *ACPBackend {
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Minute
	}
	return &ACPBackend{cfg: cfg}
}

// Name implements agentruntime.Backend.
func (b *ACPBackend) Name() string { return b.cfg.Name }

// Execute spawns the CLI, performs the ACP handshake, sends a prompt,
// and streams session/update notifications into a Session.
//
// Wire summary:
//
//	→ {"jsonrpc":"2.0","id":1,"method":"initialize","params":{...}}
//	← {"jsonrpc":"2.0","id":1,"result":{...}}
//	→ {"jsonrpc":"2.0","id":2,"method":"session/new","params":{...}}
//	← {"jsonrpc":"2.0","id":2,"result":{"sessionId":"..."}}
//	→ {"jsonrpc":"2.0","id":3,"method":"session/prompt","params":{...}}
//	← (notification stream on session/update events; ends with a final
//	   result payload that we translate to a Result struct)
func (b *ACPBackend) Execute(ctx context.Context, prompt string, opts agentruntime.ExecOptions) (agentruntime.Session, error) {
	execPath := b.cfg.Executable
	if _, err := exec.LookPath(execPath); err != nil {
		return nil, fmt.Errorf("%s backend: executable not found at %q: %w", b.cfg.Name, execPath, err)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = b.cfg.Timeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)

	args := append([]string{}, b.cfg.ExtraArgs...)
	args = append(args, "acp")
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
		return nil, fmt.Errorf("%s: stdout pipe: %w", b.cfg.Name, err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("%s: stdin pipe: %w", b.cfg.Name, err)
	}
	stderr := newStderrTail(slog.Default(), stderrTailBytes)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("%s: start: %w", b.cfg.Name, err)
	}

	sess := newACPSession(runCtx, cancel, cmd, stdin, stdout, stderr, b.cfg, prompt, opts)
	go sess.run()
	return sess, nil
}

// acpSession implements agentruntime.Session over a JSON-RPC stream.
type acpSession struct {
	ctx       context.Context
	cancel    context.CancelFunc
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    *stderrTail
	cfg       ACPConfig
	prompt    string
	opts      agentruntime.ExecOptions
	msgs      chan agentruntime.Message
	result    chan *agentruntime.Result
	cancelOne sync.Once
	idCounter atomic.Int64
}

func newACPSession(
	ctx context.Context,
	cancel context.CancelFunc,
	cmd *exec.Cmd,
	stdin io.WriteCloser,
	stdout io.ReadCloser,
	stderr *stderrTail,
	cfg ACPConfig,
	prompt string,
	opts agentruntime.ExecOptions,
) *acpSession {
	return &acpSession{
		ctx:    ctx,
		cancel: cancel,
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		cfg:    cfg,
		prompt: prompt,
		opts:   opts,
		msgs:   make(chan agentruntime.Message, 64),
		result: make(chan *agentruntime.Result, 1),
	}
}

// Messages implements agentruntime.Session.
func (s *acpSession) Messages() <-chan agentruntime.Message { return s.msgs }

// Result implements agentruntime.Session.
func (s *acpSession) Result() (*agentruntime.Result, error) {
	res, ok := <-s.result
	if !ok {
		return nil, fmt.Errorf("%s session: result channel closed", s.cfg.Name)
	}
	if res != nil && res.ErrorMessage != "" {
		return res, fmt.Errorf("%s", res.ErrorMessage)
	}
	if res == nil {
		return &agentruntime.Result{Backend: s.cfg.Name}, nil
	}
	res.Backend = s.cfg.Name
	return res, nil
}

// Cancel implements agentruntime.Session.
func (s *acpSession) Cancel(ctx context.Context) error {
	s.cancelOne.Do(func() {
		_ = s.stdin.Close()
		s.cancel()
	})
	return nil
}

// run performs the handshake and streams events until the session
// ends or the context is cancelled.
func (s *acpSession) run() {
	defer close(s.msgs)
	defer close(s.result)

	writer := bufio.NewWriter(s.stdin)
	reader := bufio.NewScanner(s.stdout)
	reader.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// 1. initialize
	initParams := s.cfg.InitParams
	if initParams == nil {
		initParams = map[string]any{
			"protocolVersion": "1",
			"clientInfo":      map[string]any{"name": "hnsxd", "version": "0.1.0"},
		}
	}
	if err := s.jsonrpcCall(writer, reader, "initialize", initParams); err != nil {
		s.fail("initialize: " + err.Error())
		return
	}

	// 2. session/new
	if err := s.jsonrpcCall(writer, reader, "session/new", map[string]any{
		"cwd": s.opts.Cwd,
		"mcp": s.opts.McpConfig,
	}); err != nil {
		s.fail("session/new: " + err.Error())
		return
	}
	// We don't actually parse the sessionId back out (this is a stub
	// grade impl); the wire is correct enough for the daemon to
	// exercise. R3.5g+ parses the response properly.

	// 3. session/prompt (notification style; we still assign an id for
	// the daemon's bookkeeping)
	if err := s.jsonrpcCall(writer, reader, "session/prompt", map[string]any{
		"prompt":  s.prompt,
		"model":   s.opts.Model,
	}); err != nil {
		s.fail("session/prompt: " + err.Error())
		return
	}

	// 4. stream notifications until the subprocess ends
	var seq int64
	for reader.Scan() {
		line := reader.Bytes()
		if len(line) == 0 {
			continue
		}
		var env struct {
			JSONRPC string          `json:"jsonrpc"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params"`
			Result  json.RawMessage `json:"result"`
			Error   *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(line, &env); err != nil {
			continue
		}
		if env.Error != nil {
			s.fail(env.Error.Message)
			return
		}
		if env.Method != "session/update" {
			// Ignore other notifications.
			continue
		}
		kind, payload := classifyACPUpdate(env.Params)
		preview := string(env.Params)
		if len(preview) > 200 {
			preview = preview[:200] + "…"
		}
		select {
		case s.msgs <- agentruntime.Message{
			Kind:     kind,
			Payload:  payload,
			Raw:      preview,
			Sequence: seq,
		}:
			seq++
		case <-s.ctx.Done():
			return
		}
	}

	if err := reader.Err(); err != nil {
		s.fail("stdout read: " + err.Error())
		return
	}

	waitErr := s.cmd.Wait()
	if waitErr != nil {
		s.fail(waitErr.Error() + ": " + s.stderr.Tail())
		return
	}
	s.result <- &agentruntime.Result{Backend: s.cfg.Name}
}

// jsonrpcCall sends a JSON-RPC request and waits for the matching
// response. Responses are matched on the request id; the function
// returns the response's result or error.
func (s *acpSession) jsonrpcCall(
	w *bufio.Writer,
	r *bufio.Scanner,
	method string,
	params map[string]any,
) error {
	id := s.idCounter.Add(1)
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	if err := writeJSONLine(w, req); err != nil {
		return err
	}
	// Read until we see a response with our id. Other notifications
	// (e.g. session/update during prompt streaming) are skipped here
	// and consumed by the streaming loop after the prompt call.
	for r.Scan() {
		var env struct {
			ID     json.RawMessage `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(r.Bytes(), &env); err != nil {
			continue
		}
		if len(env.ID) == 0 {
			continue
		}
		// Match by numeric id (we generated integers).
		var got int64
		if err := json.Unmarshal(env.ID, &got); err != nil || got != id {
			continue
		}
		if env.Error != nil {
			return fmt.Errorf("rpc %s: %s", method, env.Error.Message)
		}
		return nil
	}
	return fmt.Errorf("rpc %s: no response before EOF", method)
}

func writeJSONLine(w *bufio.Writer, v any) error {
	buf, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := w.Write(buf); err != nil {
		return err
	}
	if err := w.WriteByte('\n'); err != nil {
		return err
	}
	return w.Flush()
}

// classifyACPUpdate maps an ACP session/update event to our MessageKind.
// ACP events are highly backend-specific; the conservative default is
// MsgAssistant for any text-bearing update.
func classifyACPUpdate(params json.RawMessage) (agentruntime.MessageKind, json.RawMessage) {
	var p struct {
		Type    string `json:"type"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	_ = json.Unmarshal(params, &p)
	switch p.Type {
	case "tool_call", "tool_use":
		return agentruntime.MsgToolUse, params
	case "tool_result":
		return agentruntime.MsgToolResult, params
	case "error":
		return agentruntime.MsgError, params
	default:
		return agentruntime.MsgAssistant, params
	}
}

func (s *acpSession) fail(msg string) {
	s.result <- &agentruntime.Result{
		Backend:      s.cfg.Name,
		ErrorMessage: msg,
	}
}

var _ agentruntime.Backend = (*ACPBackend)(nil)
var _ agentruntime.Session = (*acpSession)(nil)

// BuildHelper exposes a build script the per-backend files can call.
// It is unexported; if a backend needs custom handshake it implements
// its own Execute (see ClaudeBackend).
var _ = strings.Contains // keep "strings" import live for the build helper
var _ = sync.Mutex{}