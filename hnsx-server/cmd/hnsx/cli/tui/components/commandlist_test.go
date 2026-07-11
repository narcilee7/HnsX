package components

import (
	"strings"
	"testing"

	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/common"
)

func TestCommandList_DefaultCommands(t *testing.T) {
	cl := NewCommandList(common.NewTheme())
	if got := len(cl.commands); got != 10 {
		t.Fatalf("expected 10 default commands, got %d", got)
	}
}

func TestCommandList_Filter(t *testing.T) {
	cl := NewCommandList(common.NewTheme())
	cl.SetInput("/se")
	if len(cl.filtered) != 1 {
		t.Fatalf("expected 1 match for '/se', got %d: %+v", len(cl.filtered), cl.filtered)
	}
	if cl.filtered[0].Name != "session" {
		t.Fatalf("expected first match 'session', got %s", cl.filtered[0].Name)
	}
}

func TestCommandList_Navigation(t *testing.T) {
	cl := NewCommandList(common.NewTheme())
	cl.SetInput("/")
	if cl.selected != 0 {
		t.Fatalf("expected selected=0, got %d", cl.selected)
	}
	cl.MoveDown()
	if cl.selected != 1 {
		t.Fatalf("expected selected=1, got %d", cl.selected)
	}
	cl.MoveUp()
	if cl.selected != 0 {
		t.Fatalf("expected selected=0, got %d", cl.selected)
	}
	// Should not go negative.
	cl.MoveUp()
	if cl.selected != 0 {
		t.Fatalf("expected selected=0 after top, got %d", cl.selected)
	}
}

func TestCommandList_Visible(t *testing.T) {
	cl := NewCommandList(common.NewTheme())
	cl.SetInput("/")
	if !cl.Visible() {
		t.Fatal("expected list visible for '/'")
	}
	cl.SetInput("/session ")
	if cl.Visible() {
		t.Fatal("expected list hidden once args start")
	}
	cl.SetInput("/session abc")
	if cl.Visible() {
		t.Fatal("expected list hidden when args present")
	}
}

func TestCommandList_Selected(t *testing.T) {
	cl := NewCommandList(common.NewTheme())
	cl.SetInput("/q")
	cmd, ok := cl.Selected()
	if !ok {
		t.Fatal("expected a selected command")
	}
	if cmd.Name != "quit" {
		t.Fatalf("expected quit, got %s", cmd.Name)
	}
}

func TestCommandList_View(t *testing.T) {
	cl := NewCommandList(common.NewTheme())
	cl.SetWidth(80)
	cl.SetInput("/")
	view := cl.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
	if !strings.Contains(view, "/quit") {
		t.Fatalf("expected view to contain /quit, got:\n%s", view)
	}
}

func TestCommandList_ViewEmptyWhenHidden(t *testing.T) {
	cl := NewCommandList(common.NewTheme())
	cl.SetWidth(80)
	cl.SetInput("/session abc")
	if cl.View() != "" {
		t.Fatal("expected empty view when hidden")
	}
}
