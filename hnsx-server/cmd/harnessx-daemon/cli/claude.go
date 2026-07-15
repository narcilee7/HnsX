// Package cli implements the agent subprocess layer.
//
// For W5 the package only spawns the CLI and captures stream-json output;
// later phases add the in-process Harness engine (Policy / Approval /
// Skill resolver) that runs alongside the subprocess.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/hnsx-io/hnsx/server/cmd/harnessx-daemon/wire"
)

// Invocation describes one agent CLI run.
type Invocation struct {
	Command string
	Args    []string
	Env     []string
	WorkDir string
	Prompt  string
	// OnMessage is called for every JSON line the agent emits on stdout.
	// The daemon forwards these as Multica TaskMessage frames to the server.
	OnMessage func(msg wire.TaskMessage)
	// OnProgress is called for human-readable progress lines (best-effort,
	// only when the CLI doesn't emit JSON for them).
	OnProgress func(summary string, step, total int)
}

// Run spawns the configured CLI with the supplied prompt and streams
// stdout back through the callbacks until the subprocess exits.
//
// The subprocess is killed when ctx is canceled. A non-zero exit surfaces
// as an error.
func Run(ctx context.Context, inv Invocation) error {
	if inv.Command == "" {
		return fmt.Errorf("cli.Run: empty command")
	}
	args := append([]string{}, inv.Args...)
	if inv.Prompt != "" {
		// Default: append the prompt as the final positional argument.
		// Each CLI has its own flag for this (-p, prompt, message, etc.);
		// callers should pre-shape Args accordingly. We add as final arg
		// when no --prompt-style flag is present.
		args = append(args, inv.Prompt)
	}

	cmd := exec.CommandContext(ctx, inv.Command, args...)
	cmd.Dir = inv.WorkDir
	cmd.Env = inv.Env

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// stdout: parse line-delimited JSON, forward as TaskMessage.
	wg.Add(1)
	go func() {
		defer wg.Done()
		s := bufio.NewScanner(stdout)
		s.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		seq := 0
		for s.Scan() {
			line := s.Bytes()
			if len(line) == 0 {
				continue
			}
			if inv.OnMessage != nil {
				if msg, ok := parseAgentLine(line, seq); ok {
					inv.OnMessage(msg)
					seq++
				} else if inv.OnProgress != nil {
					// Non-JSON lines become progress summaries.
					inv.OnProgress(strings.TrimSpace(string(line)), 0, 0)
				}
			}
		}
	}()

	// stderr: forward as text messages so the UI can render warnings.
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(io.Discard, stderr)
	}()

	wg.Wait()
	return cmd.Wait()
}

// parseAgentLine converts one JSONL line emitted by Claude Code / Codex into
// a Multica TaskMessage. Returns (msg, true) on a recognized line, otherwise
// (zero, false) so the caller can fall back to progress reporting.
func parseAgentLine(line []byte, seq int) (wire.TaskMessage, bool) {
	var raw struct {
		Type    string         `json:"type"`
		Role    string         `json:"role"`
		Content string         `json:"content"`
		Message json.RawMessage `json:"message"`
		// tool_use / tool_result specific shapes:
		Name   string         `json:"name"`
		Input  map[string]any `json:"input"`
		Output string         `json:"output"`
		// error specific:
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(line, &raw); err != nil {
		return wire.TaskMessage{}, false
	}

	out := wire.TaskMessage{
		Seq:       seq,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	switch raw.Type {
	case "text", "":
		if raw.Role == "assistant" || raw.Role == "user" {
			out.Type = "text"
			out.Content = raw.Content
			return out, true
		}
	case "tool_use":
		out.Type = "tool_use"
		out.Tool = raw.Name
		out.Input = raw.Input
		return out, true
	case "tool_result":
		out.Type = "tool_result"
		out.Tool = raw.Name
		out.Output = raw.Output
		return out, true
	case "error":
		out.Type = "error"
		out.Content = raw.Error.Message
		return out, true
	}

	// Fallback: treat any assistant-role JSON as a text message.
	if raw.Role == "assistant" && raw.Content != "" {
		out.Type = "text"
		out.Content = raw.Content
		return out, true
	}
	return wire.TaskMessage{}, false
}
