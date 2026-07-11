package tabs

import (
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SessionsTab is a placeholder for the full sessions list. Phase T-1 only
// renders a loading state and a hint; Phase T-2 will wire real data.
type SessionsTab struct {
	width   int
	height  int
	spinner spinner.Model
}

// NewSessionsTab creates a new sessions tab.
func NewSessionsTab() SessionsTab {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return SessionsTab{spinner: s}
}

// Init starts the spinner.
func (t SessionsTab) Init() tea.Cmd {
	return t.spinner.Tick
}

// Update handles messages for the sessions tab.
func (t SessionsTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
	default:
		var cmd tea.Cmd
		t.spinner, cmd = t.spinner.Update(msg)
		return t, cmd
	}
	return t, nil
}

// View renders the placeholder sessions view.
func (t SessionsTab) View() string {
	if t.width < 1 {
		return "Loading sessions..."
	}
	line := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9C9C92")).
		Render(t.spinner.View() + " Sessions list will appear here (Phase T-2)")
	return lipgloss.NewStyle().
		Width(t.width).
		Height(t.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(line)
}

// SetSize updates the tab size.
func (t *SessionsTab) SetSize(width, height int) {
	t.width = width
	t.height = height
}
