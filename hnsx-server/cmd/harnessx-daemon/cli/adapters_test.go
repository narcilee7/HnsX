package cli

import (
	"strings"
	"testing"
)

func TestBuildInvocation_Claude(t *testing.T) {
	cmd, args, env := BuildInvocation(AdapterClaude, AdapterOptions{Model: "claude-haiku-4-5"})
	if cmd != "claude" {
		t.Fatalf("command = %q, want claude", cmd)
	}
	if !contains(args, "--output-format") || !contains(args, "stream-json") {
		t.Fatalf("expected stream-json flag; got %v", args)
	}
	if !contains(args, "--model") || !contains(args, "claude-haiku-4-5") {
		t.Fatalf("expected --model flag; got %v", args)
	}
	if len(env) != 0 {
		t.Fatalf("expected empty env; got %v", env)
	}
}

func TestBuildInvocation_Codex(t *testing.T) {
	cmd, args, _ := BuildInvocation(AdapterCodex, AdapterOptions{Prompt: "hi"})
	if cmd != "codex" {
		t.Fatalf("command = %q, want codex", cmd)
	}
	if !contains(args, "exec") || !contains(args, "--json") {
		t.Fatalf("expected exec --json; got %v", args)
	}
	if !contains(args, "hi") {
		t.Fatalf("expected prompt as final arg; got %v", args)
	}
}

func TestBuildInvocation_AllHavePermissionMode(t *testing.T) {
	for _, a := range []Adapter{AdapterClaude, AdapterCodex, AdapterCodeBuddy, AdapterCopilot, AdapterCursor} {
		_, args, _ := BuildInvocation(a, AdapterOptions{})
		if len(args) == 0 {
			t.Errorf("%s: empty args", a)
		}
	}
}

func TestValidateAdapter(t *testing.T) {
	if err := ValidateAdapter("claude"); err != nil {
		t.Errorf("expected claude to validate; got %v", err)
	}
	if err := ValidateAdapter("unknown-agent"); err == nil {
		t.Errorf("expected unknown to fail")
	}
}

func TestDetectAdapterFromBinary(t *testing.T) {
	cases := map[string]Adapter{
		"claude":                  AdapterClaude,
		"/usr/local/bin/claude":   AdapterClaude,
		"codex":                   AdapterCodex,
		"codebuddy":               AdapterCodeBuddy,
		"copilot":                 AdapterCopilot,
		"cursor-agent":            AdapterCursor,
		"opencode":                AdapterOpenCode,
		"openclaw":                AdapterOpenClaw,
		"hermes":                  AdapterHermes,
		"pi":                      AdapterPi,
		"kimi":                    AdapterKimi,
		"kiro":                    AdapterKiro,
		"qoder":                   AdapterQoder,
		"traecli":                 AdapterTrae,
		"agy":                     AdapterAntigravity,
		"unknown-tool":            "",
	}
	for in, want := range cases {
		if got := detectAdapterFromBinary(in); got != want {
			t.Errorf("detectAdapterFromBinary(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAllAdapters_Count(t *testing.T) {
	if got := len(AllAdapters()); got < 13 {
		t.Errorf("expected >= 13 adapters; got %d", got)
	}
}

func contains(s []string, sub string) bool {
	for _, x := range s {
		if strings.Contains(x, sub) {
			return true
		}
	}
	return false
}
