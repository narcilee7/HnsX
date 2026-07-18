package daemon_runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/hnsx-io/hnsx/server/internal/domain/agent"
	"github.com/hnsx-io/hnsx/server/internal/domain/agentruntime"
)

// hashPrompt returns the hex sha256 digest of the rendered prompt.
// Lets eval slice regressions by exact prompt text.
func hashPrompt(p string) string {
	sum := sha256.Sum256([]byte(p))
	return hex.EncodeToString(sum[:])
}

// agentTemplateID derives a stable template identifier from the agent's
// runtime config. R3.x promotes this to a first-class AgentTemplate
// entity; for now we use a.ID.
func agentTemplateID(a *agent.Agent, backend, model string) string {
	if a == nil {
		return backend + ":" + model
	}
	return a.ID
}

// toolSignatureSet accumulates tool names seen during an agent run,
// preserving insertion order and emitting a stable JSON array.
type toolSignatureSet struct {
	order []string
	set   map[string]struct{}
}

func newToolSignatureSet() *toolSignatureSet {
	return &toolSignatureSet{set: make(map[string]struct{})}
}

func (s *toolSignatureSet) Add(name string) {
	if name == "" {
		return
	}
	if _, ok := s.set[name]; ok {
		return
	}
	s.set[name] = struct{}{}
	s.order = append(s.order, name)
}

func (s *toolSignatureSet) JSON() json.RawMessage {
	if len(s.order) == 0 {
		return json.RawMessage("[]")
	}
	buf, _ := json.Marshal(s.order)
	return buf
}

// extractToolName pulls the tool name out of a top-level tool_use
// message. Returns "" if the message is not a tool_use event.
func extractToolName(m agentruntime.Message) string {
	if m.Kind != agentruntime.MsgToolUse {
		return ""
	}
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(m.Payload, &p); err != nil {
		return ""
	}
	return strings.TrimSpace(p.Name)
}

// toolNamesForMessage returns every tool_name implied by the message,
// regardless of whether tool_use is a top-level event or embedded
// inside an assistant message's content array (Claude's stream-json).
//
// Returns an empty slice when the message contains no tool invocations.
func toolNamesForMessage(m agentruntime.Message) []string {
	if name := extractToolName(m); name != "" {
		return []string{name}
	}
	if m.Kind != agentruntime.MsgAssistant {
		return nil
	}
	// Parse the assistant message's content[] for tool_use blocks.
	var parsed struct {
		Message struct {
			Content []struct {
				Type  string          `json:"type"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(m.Payload, &parsed); err != nil {
		return nil
	}
	var out []string
	for _, c := range parsed.Message.Content {
		if c.Type == "tool_use" && c.Name != "" {
			out = append(out, c.Name)
		}
	}
	return out
}