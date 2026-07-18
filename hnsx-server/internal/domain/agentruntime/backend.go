// Package agentruntime defines the port for spawning agent CLIs.
//
// A Backend turns a (prompt, opts) pair into a streaming Session that emits
// Messages and eventually a Result. Implementations live in
// internal/infra/agentruntime/ (claude.go, codex.go, cursor.go, ...).
//
// Domain layer defines the contract; infrastructure layer implements it.
// This keeps the service layer free of subprocess / stream-json concerns.
package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Backend is the port every CLI agent implements.
type Backend interface {
	// Name returns the canonical backend identifier (e.g. "claude", "codex").
	Name() string
	// Execute spawns the agent subprocess and returns a Session bound to its
	// message stream. Caller is responsible for closing the Session.
	Execute(ctx context.Context, prompt string, opts ExecOptions) (*Session, error)
}

// Session is the live handle to a running agent.
type Session interface {
	// Messages yields parsed agent messages until the session ends or ctx is
	// cancelled. The channel is closed by the implementation when the
	// underlying subprocess exits.
	Messages() <-chan Message
	// Result blocks until the session ends and returns the final result.
	// Returns an error if the session was cancelled or the subprocess
	// exited with an error.
	Result() (*Result, error)
	// Cancel signals the subprocess to terminate. Safe to call multiple times.
	Cancel(ctx context.Context) error
}

// MessageKind discriminates message types emitted by an agent.
type MessageKind string

const (
	MsgAssistant  MessageKind = "assistant"
	MsgToolUse    MessageKind = "tool_use"
	MsgToolResult MessageKind = "tool_result"
	MsgProgress   MessageKind = "progress"
	MsgError      MessageKind = "error"
	MsgSystem     MessageKind = "system"
)

// Message is a normalized agent event. Backends translate their wire
// formats (stream-json for Claude/Codex, native for Cursor, ...) into this
// shape so the rest of HnsX is wire-format agnostic.
type Message struct {
	Kind     MessageKind
	Payload  json.RawMessage // backend-specific structured body
	Raw      string          // human-readable preview for live debug
	Sequence int64
}

// Result captures the final outcome of a session.
type Result struct {
	Backend       string
	ExitCode      int
	Summary       string
	TokensIn      int64
	TokensOut     int64
	CostUSD       float64
	DurationMs    int64
	ErrorMessage  string
}

// ExecOptions tunes a single Execute call.
type ExecOptions struct {
	Cwd          string
	Model        string
	Timeout      time.Duration
	McpConfig    json.RawMessage // optional MCP overlay
	SystemPrompt string
	ExtraEnv     map[string]string
	ExtraArgs    []string
}

// ErrBackendNotFound is returned by Registry when no backend matches the name.
var ErrBackendNotFound = errors.New("agentruntime: backend not found")

// Registry resolves a backend by name. Implemented in infra/agentruntime.
type Registry interface {
	Get(name string) (Backend, error)
	List() []string
}

// Time helpers are exposed via the time package; ExecOptions.Timeout uses it.