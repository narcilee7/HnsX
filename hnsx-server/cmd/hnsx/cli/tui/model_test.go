package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewModel(t *testing.T) {
	m := NewModel("http://127.0.0.1:50052")
	if len(m.tabs) != len(tabNames) {
		t.Fatalf("tabs = %d, want %d", len(m.tabs), len(tabNames))
	}
	if m.activeTab != 0 {
		t.Fatalf("activeTab = %d, want 0", m.activeTab)
	}
}

func TestModel_NextPrevTab(t *testing.T) {
	m := NewModel("http://127.0.0.1:50052")
	m.width = 100
	m.height = 30

	m = updateModel(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.activeTab != 1 {
		t.Fatalf("after tab: activeTab = %d, want 1", m.activeTab)
	}

	m = updateModel(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.activeTab != 0 {
		t.Fatalf("after shift+tab: activeTab = %d, want 0", m.activeTab)
	}
}

func TestModel_NumberTabs(t *testing.T) {
	m := NewModel("http://127.0.0.1:50052")
	m.width = 100
	m.height = 30

	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	if m.activeTab != 2 {
		t.Fatalf("after '3': activeTab = %d, want 2", m.activeTab)
	}
}

func TestModel_HelpToggle(t *testing.T) {
	m := NewModel("http://127.0.0.1:50052")
	m.width = 100
	m.height = 30

	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !m.helpOpen {
		t.Fatal("expected help to open after '?'")
	}

	m = updateModel(m, tea.KeyMsg{Type: tea.KeyEscape})
	if m.helpOpen {
		t.Fatal("expected help to close after esc")
	}
}

func TestModel_Quit(t *testing.T) {
	m := NewModel("http://127.0.0.1:50052")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit command after 'q'")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestModel_HealthUpdate(t *testing.T) {
	m := NewModel("http://127.0.0.1:1")
	m.serverOK = true
	m = updateModel(m, healthMsg{ok: false})
	if m.serverOK {
		t.Fatal("expected serverOK to become false")
	}
}

func TestModel_TickTriggersHealthCheck(t *testing.T) {
	m := NewModel("http://127.0.0.1:1")
	_, cmd := m.Update(tickMsg{})
	if cmd == nil {
		t.Fatal("expected cmd after tick")
	}
	// The command is a batch; we just verify it exists.
}

func TestModel_ViewNonZero(t *testing.T) {
	m := NewModel("http://127.0.0.1:50052")
	m.width = 120
	m.height = 40
	view := m.View()
	if view == "" {
		t.Fatal("View() should not be empty for positive dimensions")
	}
}

func updateModel(m Model, msg tea.Msg) Model {
	out, _ := m.Update(msg)
	return out.(Model)
}
