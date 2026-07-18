package agentruntime

import (
	"testing"

	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
)

func TestDecodeClaudeStreamLine_Assistant(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"id":"msg_1","content":[{"type":"text","text":"hello"}]}}`)
	msg, ok := decodeClaudeStreamLine(line, 0)
	if !ok {
		t.Fatal("expected ok=true for assistant event")
	}
	if msg.Kind != agentruntime.MsgAssistant {
		t.Errorf("kind = %q, want %q", msg.Kind, agentruntime.MsgAssistant)
	}
	if msg.Sequence != 0 {
		t.Errorf("sequence = %d, want 0", msg.Sequence)
	}
}

func TestDecodeClaudeStreamLine_ToolUse(t *testing.T) {
	line := []byte(`{"type":"tool_use","name":"Read","input":{"path":"/etc/hosts"}}`)
	msg, ok := decodeClaudeStreamLine(line, 7)
	if !ok {
		t.Fatal("expected ok=true for tool_use event")
	}
	if msg.Kind != agentruntime.MsgToolUse {
		t.Errorf("kind = %q, want %q", msg.Kind, agentruntime.MsgToolUse)
	}
	if msg.Sequence != 7 {
		t.Errorf("sequence = %d, want 7", msg.Sequence)
	}
}

func TestDecodeClaudeStreamLine_UnknownType(t *testing.T) {
	line := []byte(`{"type":"future_event_we_dont_know","foo":"bar"}`)
	if _, ok := decodeClaudeStreamLine(line, 0); ok {
		t.Fatal("expected ok=false for unknown event type")
	}
}

func TestDecodeClaudeStreamLine_InvalidJSON(t *testing.T) {
	line := []byte(`not json at all`)
	if _, ok := decodeClaudeStreamLine(line, 0); ok {
		t.Fatal("expected ok=false for malformed JSON")
	}
}

func TestExtractClaudeResult_Success(t *testing.T) {
	line := []byte(`{"type":"result","subtype":"success","duration_ms":1234,"total_cost_usd":0.012,"usage":{"input_tokens":100,"output_tokens":50},"result":"hello"}`)
	r := extractClaudeResult(line)
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	if r.DurationMs != 1234 {
		t.Errorf("DurationMs = %d, want 1234", r.DurationMs)
	}
	if r.CostUSD != 0.012 {
		t.Errorf("CostUSD = %v, want 0.012", r.CostUSD)
	}
	if r.TokensIn != 100 {
		t.Errorf("TokensIn = %d, want 100", r.TokensIn)
	}
	if r.TokensOut != 50 {
		t.Errorf("TokensOut = %d, want 50", r.TokensOut)
	}
	if r.Summary != "hello" {
		t.Errorf("Summary = %q, want %q", r.Summary, "hello")
	}
	if r.ErrorMessage != "" {
		t.Errorf("ErrorMessage = %q, want empty", r.ErrorMessage)
	}
}

func TestExtractClaudeResult_Error(t *testing.T) {
	line := []byte(`{"type":"result","subtype":"error","is_error":true,"result":"rate limited"}`)
	r := extractClaudeResult(line)
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	if r.ErrorMessage == "" {
		t.Error("ErrorMessage should be non-empty for is_error=true")
	}
}

func TestExtractClaudeResult_NonResultLine(t *testing.T) {
	line := []byte(`{"type":"assistant","message":{"id":"x"}}`)
	if r := extractClaudeResult(line); r != nil {
		t.Errorf("expected nil for non-result line, got %+v", r)
	}
}