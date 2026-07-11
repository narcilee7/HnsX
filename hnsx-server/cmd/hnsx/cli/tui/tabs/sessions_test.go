package tabs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hnsx-io/hnsx/server/internal/client"
)

func TestSessionsTab_Init(t *testing.T) {
	tab := NewSessionsTab("http://127.0.0.1:1")
	cmd := tab.Init()
	if cmd == nil {
		t.Fatal("expected non-nil Init cmd")
	}
	// Init returns a batch containing fetch + tick.
	if _, ok := cmd().(tea.BatchMsg); !ok {
		t.Fatalf("expected tea.BatchMsg from Init, got %T", cmd())
	}
}

func TestSessionsTab_Navigation(t *testing.T) {
	tab := NewSessionsTab("http://127.0.0.1:1")
	tab.sessions = []client.SessionListItem{
		{ID: "s1", DomainID: "d1", State: "running"},
		{ID: "s2", DomainID: "d2", State: "completed"},
	}
	tab.filtered = tab.sessions
	tab.width = 100
	tab.height = 30

	tab = updateSessionsTab(tab, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if tab.selected != 1 {
		t.Fatalf("selected = %d, want 1", tab.selected)
	}
	tab = updateSessionsTab(tab, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if tab.selected != 0 {
		t.Fatalf("selected = %d, want 0", tab.selected)
	}
}

func TestTracesTab_Fetch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"trace_id": "t1", "session_id": "s1", "domain_id": "d1", "total_cost_usd": 0.001},
			},
		})
	}))
	defer ts.Close()

	tab := NewTracesTab(ts.URL)
	cmd := tab.fetchTraces()
	msg := cmd()
	loaded, ok := msg.(tracesLoadedMsg)
	if !ok {
		t.Fatalf("expected tracesLoadedMsg, got %T", msg)
	}
	if loaded.err != nil {
		t.Fatalf("unexpected error: %v", loaded.err)
	}
	if len(loaded.items) != 1 {
		t.Fatalf("items = %d, want 1", len(loaded.items))
	}
}

func TestTracesTab_Init(t *testing.T) {
	tab := NewTracesTab("http://127.0.0.1:1")
	cmd := tab.Init()
	if cmd == nil {
		t.Fatal("expected non-nil Init cmd")
	}
	if _, ok := cmd().(tea.BatchMsg); !ok {
		t.Fatalf("expected tea.BatchMsg from Init, got %T", cmd())
	}
}

func updateSessionsTab(tab SessionsTab, msg tea.Msg) SessionsTab {
	out, _ := tab.Update(msg)
	return out.(SessionsTab)
}
