package tabs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestAuditTab_FilterPrompt(t *testing.T) {
	tab := NewAuditTab("http://127.0.0.1:1")
	tab = updateAuditTab(tab, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if !tab.filtering {
		t.Fatal("expected filter prompt after 'f'")
	}
}

func updateAuditTab(tab AuditTab, msg tea.Msg) AuditTab {
	out, _ := tab.Update(msg)
	return out.(AuditTab)
}

func TestDomainsTab_Init(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"domains": []map[string]any{
				{"id": "d1", "version": "v1", "status": "active"},
			},
		})
	}))
	defer ts.Close()

	tab := NewDomainsTab(ts.URL)
	cmd := tab.Init()
	if _, ok := cmd().(tea.BatchMsg); !ok {
		t.Fatalf("expected tea.BatchMsg from Init, got %T", cmd())
	}
}

func TestDashboardTab_Init(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/sessions":
			_ = json.NewEncoder(w).Encode(map[string]any{"sessions": []map[string]any{}})
		case "/api/v1/traces":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	tab := NewDashboardTab(ts.URL)
	cmd := tab.Init()
	if _, ok := cmd().(tea.BatchMsg); !ok {
		t.Fatalf("expected tea.BatchMsg from Init, got %T", cmd())
	}
}
