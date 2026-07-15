package cli

import (
	"context"
	"fmt"
	"strings"
)

// Adapter knows how to spawn one agent CLI. Each agent (Claude Code, Codex,
// CodeBuddy, GitHub Copilot CLI, Cursor Agent) has its own argv conventions
// for stream-json output; we encapsulate them here.
//
// The five adapters below are direct ports of the upstream Multica fork's
// buildClaudeArgs / buildCodexArgs / buildCodebuddyArgs / buildCopilotArgs
// / buildCursorArgs from multica_fork/pkg/agent/, trimmed to the spawn
// flags we need.
//
// Adding a new agent: append a new buildXxxArgs function + a case to
// BuildInvocation. The fork's adapter package has the canonical versions
// for any new agent — copy the buildXxxArgs body and adjust.
type Adapter string

const (
	AdapterClaude      Adapter = "claude"
	AdapterCodex       Adapter = "codex"
	AdapterCodeBuddy   Adapter = "codebuddy"
	AdapterCopilot     Adapter = "copilot"
	AdapterCursor      Adapter = "cursor"
	AdapterOpenCode    Adapter = "opencode"
	AdapterOpenClaw    Adapter = "openclaw"
	AdapterHermes      Adapter = "hermes"
	AdapterPi          Adapter = "pi"
	AdapterKimi        Adapter = "kimi"
	AdapterKiro        Adapter = "kiro"
	AdapterQoder       Adapter = "qoder"
	AdapterTrae        Adapter = "traecli"
	AdapterAntigravity Adapter = "antigravity"
)

// AllAdapters returns every supported Adapter. Used by auto-detect at
// daemon startup so a single binary covers all 13+ agent CLIs.
func AllAdapters() []Adapter {
	return []Adapter{
		AdapterClaude, AdapterCodex, AdapterCodeBuddy, AdapterCopilot,
		AdapterCursor, AdapterOpenCode, AdapterOpenClaw, AdapterHermes,
		AdapterPi, AdapterKimi, AdapterKiro, AdapterQoder, AdapterTrae,
		AdapterAntigravity,
	}
}

// AdapterOptions configures how the agent is invoked. The zero value is
// acceptable: stream-json, bypass-permissions, default model.
type AdapterOptions struct {
	Model        string // override the default model
	WorkingDir   string // cwd for the subprocess
	ExtraEnv     []string // merged into the subprocess env
	Prompt       string // the user/task prompt
}

// BuildInvocation returns the (command, args, env) triple to spawn the
// given adapter. The command is the CLI binary name; the caller resolves
// it via PATH. env is the merged environment.
func BuildInvocation(a Adapter, opts AdapterOptions) (cmd string, args []string, env []string) {
	switch a {
	case AdapterClaude:
		return "claude", buildClaudeArgs(opts), opts.ExtraEnv
	case AdapterCodex:
		return "codex", buildCodexArgs(opts), opts.ExtraEnv
	case AdapterCodeBuddy:
		return "codebuddy", buildCodeBuddyArgs(opts), opts.ExtraEnv
	case AdapterCopilot:
		return "copilot", buildCopilotArgs(opts), opts.ExtraEnv
	case AdapterCursor:
		return "cursor-agent", buildCursorArgs(opts), opts.ExtraEnv
	case AdapterOpenCode:
		return "opencode", buildOpenCodeArgs(opts), opts.ExtraEnv
	case AdapterOpenClaw:
		return "openclaw", buildOpenClawArgs(opts), opts.ExtraEnv
	case AdapterHermes:
		return "hermes", buildHermesArgs(opts), opts.ExtraEnv
	case AdapterPi:
		return "pi", buildPiArgs(opts), opts.ExtraEnv
	case AdapterKimi:
		return "kimi", buildKimiArgs(opts), opts.ExtraEnv
	case AdapterKiro:
		return "kiro", buildKiroArgs(opts), opts.ExtraEnv
	case AdapterQoder:
		return "qoder", buildQoderArgs(opts), opts.ExtraEnv
	case AdapterTrae:
		return "traecli", buildTraeArgs(opts), opts.ExtraEnv
	case AdapterAntigravity:
		return "agy", buildAntigravityArgs(opts), opts.ExtraEnv
	}
	return string(a), []string{}, opts.ExtraEnv
}

// buildClaudeArgs matches Multica's claude.go stream-json invocation.
func buildClaudeArgs(opts AdapterOptions) []string {
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--verbose",
		"--permission-mode", "bypassPermissions",
		// AskUserQuestion is interactive; daemon has no UI for it.
		"--disallowedTools", "AskUserQuestion",
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// buildCodexArgs matches Multica's codex.go exec invocation.
func buildCodexArgs(opts AdapterOptions) []string {
	args := []string{"exec", "--json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// buildCodeBuddyArgs for CodeBuddy's CLI.
func buildCodeBuddyArgs(opts AdapterOptions) []string {
	args := []string{"chat", "--output-format", "stream-json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// buildCopilotArgs for GitHub Copilot CLI.
func buildCopilotArgs(opts AdapterOptions) []string {
	args := []string{"-p", "--output-format", "stream-json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// buildCursorArgs for Cursor Agent CLI.
func buildCursorArgs(opts AdapterOptions) []string {
	args := []string{"agent", "--output-format", "stream-json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// buildOpenCodeArgs for OpenCode CLI.
func buildOpenCodeArgs(opts AdapterOptions) []string {
	args := []string{"run", "--format", "json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// buildOpenClawArgs for OpenClaw CLI.
func buildOpenClawArgs(opts AdapterOptions) []string {
	args := []string{"exec", "--json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// buildHermesArgs for Hermes CLI.
func buildHermesArgs(opts AdapterOptions) []string {
	args := []string{"--json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// buildPiArgs for Pi CLI.
func buildPiArgs(opts AdapterOptions) []string {
	args := []string{"-p", "--output", "json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// buildKimiArgs for Kimi CLI (Moonshot).
func buildKimiArgs(opts AdapterOptions) []string {
	args := []string{"--stream-json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// buildKiroArgs for Kiro CLI (AWS).
func buildKiroArgs(opts AdapterOptions) []string {
	args := []string{"chat", "--output-format", "stream-json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// buildQoderArgs for Qoder CLI.
func buildQoderArgs(opts AdapterOptions) []string {
	args := []string{"run", "--json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// buildTraeArgs for Trae CLI.
func buildTraeArgs(opts AdapterOptions) []string {
	args := []string{"chat", "--stream"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// buildAntigravityArgs for Antigravity CLI (binary: agy).
func buildAntigravityArgs(opts AdapterOptions) []string {
	args := []string{"run", "--output-format", "stream-json"}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.Prompt != "" {
		args = append(args, opts.Prompt)
	}
	return args
}

// ValidateAdapter returns nil iff a is a known adapter name.
func ValidateAdapter(a string) error {
	for _, known := range AllAdapters() {
		if string(known) == a {
			return nil
		}
	}
	names := make([]string, 0, len(AllAdapters()))
	for _, a := range AllAdapters() {
		names = append(names, string(a))
	}
	return fmt.Errorf("unknown adapter: %s (supported: %s)", a, strings.Join(names, ", "))
}

// detectAdapterFromBinary inspects the binary name/path and returns the
// matching Adapter. Used when the daemon auto-detects CLIs on PATH at
// startup.
//
// Matches the Multica fork's agent_supported_types list (13 agents total).
// Returning an empty string means "unrecognized".
func detectAdapterFromBinary(binPath string) Adapter {
	base := binPath
	if i := strings.LastIndexAny(base, "/\\"); i >= 0 {
		base = base[i+1:]
	}
	switch base {
	case "claude", "claude-code":
		return AdapterClaude
	case "codex":
		return AdapterCodex
	case "codebuddy":
		return AdapterCodeBuddy
	case "copilot", "gh-copilot":
		return AdapterCopilot
	case "cursor-agent", "cursor":
		return AdapterCursor
	case "opencode":
		return AdapterOpenCode
	case "openclaw":
		return AdapterOpenClaw
	case "hermes":
		return AdapterHermes
	case "pi":
		return AdapterPi
	case "kimi":
		return AdapterKimi
	case "kiro", "kiro-cli":
		return AdapterKiro
	case "qoder", "qodercli":
		return AdapterQoder
	case "trae", "traecli":
		return AdapterTrae
	case "agy", "antigravity":
		return AdapterAntigravity
	}
	return ""
}

// spawnAdapter is a small helper around BuildInvocation + Run that lets
// callers spawn any of the five adapters without constructing the
// Invocation struct manually.
func spawnAdapter(ctx context.Context, a Adapter, opts AdapterOptions, onMessage func(string), onProgress func(string, int, int)) error {
	cmd, args, env := BuildInvocation(a, opts)
	inv := Invocation{
		Command:    cmd,
		Args:       args,
		Env:        env,
		WorkDir:    opts.WorkingDir,
		Prompt:     "", // already embedded in args
		OnMessage:  nil, // adapter does its own stdout parsing; we just notify
		OnProgress: onProgress,
	}
	_ = inv
	_ = onMessage
	return Run(ctx, Invocation{
		Command: cmd, Args: args, Env: env, WorkDir: opts.WorkingDir,
		OnProgress: onProgress,
	})
}
