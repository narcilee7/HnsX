package daemon_runtime

import (
	"encoding/json"
	"testing"

	"github.com/hnsx-io/hnsx/server/internal/domain/agent"
	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
)

func TestHashPrompt_Deterministic(t *testing.T) {
	a := hashPrompt("hello world")
	b := hashPrompt("hello world")
	if a != b {
		t.Errorf("same input produced different hashes: %s vs %s", a, b)
	}
	if len(a) != 64 {
		t.Errorf("expected 64 hex chars (sha256), got %d: %q", len(a), a)
	}
}

func TestHashPrompt_DifferentInputsDifferentHashes(t *testing.T) {
	a := hashPrompt("hello")
	b := hashPrompt("hello world")
	if a == b {
		t.Errorf("expected different hashes for different prompts")
	}
}

func TestHashPrompt_KnownVector(t *testing.T) {
	// sha256("hello") = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
	got := hashPrompt("hello")
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Errorf("hashPrompt(hello) = %s, want %s", got, want)
	}
}

func TestAgentTemplateID(t *testing.T) {
	a := &agent.Agent{ID: "ag-123"}
	got := agentTemplateID(a, "claude", "MiniMax-M3")
	if got != "ag-123" {
		t.Errorf("agentTemplateID = %q, want ag-123", got)
	}
}

func TestAgentTemplateID_NilAgent(t *testing.T) {
	got := agentTemplateID(nil, "codex", "o3")
	if got != "codex:o3" {
		t.Errorf("nil agent fallback = %q, want codex:o3", got)
	}
}

func TestToolSignatureSet_AddAndJSON(t *testing.T) {
	s := newToolSignatureSet()
	s.Add("Read")
	s.Add("Bash")
	s.Add("Read") // dedup
	s.Add("Write")
	out := string(s.JSON())
	if out != `["Read","Bash","Write"]` {
		t.Errorf("ToolSignatures JSON = %s, want [Read,Bash,Write]", out)
	}
}

func TestToolSignatureSet_Empty(t *testing.T) {
	s := newToolSignatureSet()
	out := string(s.JSON())
	if out != "[]" {
		t.Errorf("empty ToolSignatures JSON = %s, want []", out)
	}
}

func TestExtractToolName_ToolUse(t *testing.T) {
	m := agentruntime.Message{
		Kind: agentruntime.MsgToolUse,
		Payload: json.RawMessage(`{"type":"tool_use","name":"Read","input":{"path":"/tmp/x"}}`),
	}
	if got := extractToolName(m); got != "Read" {
		t.Errorf("extractToolName = %q, want Read", got)
	}
}

func TestExtractToolName_NotToolUse(t *testing.T) {
	m := agentruntime.Message{
		Kind:    agentruntime.MsgAssistant,
		Payload: json.RawMessage(`{"type":"assistant","message":{"id":"m1"}}`),
	}
	if got := extractToolName(m); got != "" {
		t.Errorf("extractToolName on assistant = %q, want empty", got)
	}
}

func TestExtractToolName_InvalidJSON(t *testing.T) {
	m := agentruntime.Message{
		Kind:    agentruntime.MsgToolUse,
		Payload: json.RawMessage(`{not json`),
	}
	if got := extractToolName(m); got != "" {
		t.Errorf("extractToolName on bad json = %q, want empty", got)
	}
}