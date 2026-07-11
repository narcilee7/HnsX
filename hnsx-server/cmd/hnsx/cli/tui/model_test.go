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

func TestModel_CommandMode(t *testing.T) {
	m := NewModel("http://127.0.0.1:50052")
	m.width = 100
	m.height = 30

	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !m.commandMode {
		t.Fatal("expected command mode after '/'")
	}
	if m.commandInput.Value() != "/" {
		t.Fatalf("expected input '/', got %q", m.commandInput.Value())
	}
	if !m.commandList.Visible() {
		t.Fatal("expected command list visible after '/'")
	}

	m = updateModel(m, tea.KeyMsg{Type: tea.KeyEscape})
	if m.commandMode {
		t.Fatal("expected command mode to close after esc")
	}
}

func TestModel_CommandListFilterAndSelect(t *testing.T) {
	m := NewModel("http://127.0.0.1:50052")
	m.width = 100
	m.height = 30

	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if !m.commandList.Visible() {
		t.Fatal("expected visible filtered list")
	}
	if cmd, ok := m.commandList.Selected(); !ok || cmd.Name != "quit" {
		t.Fatalf("expected quit selected, got %+v", cmd)
	}

	out, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := out.(Model)
	if cmd == nil {
		t.Fatal("expected command after selecting quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
	if mm.commandMode {
		t.Fatal("expected command mode to exit after executing quit")
	}
}

func TestModel_CommandListFillArgCommand(t *testing.T) {
	m := NewModel("http://127.0.0.1:50052")
	m.width = 100
	m.height = 30

	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	out, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := out.(Model)
	if cmd == nil {
		t.Fatal("expected command after selecting arg command")
	}
	if mm.commandInput.Value() != "/session " {
		t.Fatalf("expected input '/session ', got %q", mm.commandInput.Value())
	}
	if mm.commandList.Visible() {
		t.Fatal("expected command list hidden when typing args")
	}
}

func TestModel_CommandListNavigation(t *testing.T) {
	m := NewModel("http://127.0.0.1:50052")
	m.width = 100
	m.height = 30

	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if cmd, ok := m.commandList.Selected(); !ok || cmd.Name != "help" {
		t.Fatalf("expected 'help' selected initially, got %+v", cmd)
	}
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyDown})
	if cmd, ok := m.commandList.Selected(); !ok || cmd.Name != "quit" {
		t.Fatalf("expected 'quit' selected after down, got %+v", cmd)
	}
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyUp})
	if cmd, ok := m.commandList.Selected(); !ok || cmd.Name != "help" {
		t.Fatalf("expected 'help' selected after up, got %+v", cmd)
	}
}

func TestModel_CommandQuit(t *testing.T) {
	m := NewModel("http://127.0.0.1:50052")
	m.width = 100
	m.height = 30

	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = updateModel(m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	out, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := out.(Model)
	if cmd == nil {
		t.Fatal("expected command after enter")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", cmd())
	}
	if mm.commandMode {
		t.Fatal("expected command mode to exit after quit")
	}
}

func TestParseCommand(t *testing.T) {
	cmd := parseCommand("/session abc123")
	if cmd.name != "session" || len(cmd.args) != 1 || cmd.args[0] != "abc123" {
		t.Fatalf("parse failed: %+v", cmd)
	}

	cmd = parseCommand("reject id1 reason=foo")
	if cmd.name != "reject" || len(cmd.args) != 1 || cmd.kwargs["reason"] != "foo" {
		t.Fatalf("parse kwargs failed: %+v", cmd)
	}
}

func updateModel(m Model, msg tea.Msg) Model {
	out, _ := m.Update(msg)
	return out.(Model)
}
