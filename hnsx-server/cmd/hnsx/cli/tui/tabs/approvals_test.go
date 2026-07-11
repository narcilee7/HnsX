package tabs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/common"
)

func TestApprovalsTab_Init(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "a1", "session_id": "s1", "risk": "high"},
		})
	}))
	defer ts.Close()

	tab := NewApprovalsTab(ts.URL)
	cmd := tab.Init()
	if _, ok := cmd().(tea.BatchMsg); !ok {
		t.Fatalf("expected tea.BatchMsg from Init, got %T", cmd())
	}
}

func TestApprovalsTab_RejectPrompt(t *testing.T) {
	tab := NewApprovalsTab("http://127.0.0.1:1")
	tab.approvals = []common.ApprovalItem{{ID: "a1", SessionID: "s1", Risk: "high"}}
	tab = updateApprovalsTab(tab, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if !tab.rejecting {
		t.Fatal("expected rejecting prompt after 'x'")
	}
}

func updateApprovalsTab(tab ApprovalsTab, msg tea.Msg) ApprovalsTab {
	out, _ := tab.Update(msg)
	return out.(ApprovalsTab)
}

func TestEvalTab_Init(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"set_id": "e1", "domain_id": "d1", "case_count": 3},
			},
		})
	}))
	defer ts.Close()

	tab := NewEvalTab(ts.URL)
	cmd := tab.Init()
	msg := cmd()
	loaded, ok := msg.(evalSetsLoadedMsg)
	if !ok {
		t.Fatalf("expected evalSetsLoadedMsg, got %T", msg)
	}
	if loaded.err != nil {
		t.Fatalf("unexpected error: %v", loaded.err)
	}
	if len(loaded.items) != 1 {
		t.Fatalf("items = %d, want 1", len(loaded.items))
	}
}
