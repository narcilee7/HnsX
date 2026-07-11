package tabs

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/common"
	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/components"
)

// AuditTab shows audit log entries with actor/resource filtering.
type AuditTab struct {
	client *common.Client

	width  int
	height int
	theme  common.Theme

	entries  []common.AuditItem
	filtered []common.AuditItem
	selected int
	err      error

	filtering bool
	filter    textinput.Model
}

// NewAuditTab creates an audit tab.
func NewAuditTab(serverURL string) AuditTab {
	th := common.NewTheme()
	ti := textinput.New()
	ti.Placeholder = "actor:foo resource:bar"
	return AuditTab{
		client: common.NewClient(serverURL),
		theme:  th,
		filter: ti,
	}
}

// Init starts polling.
func (t AuditTab) Init() tea.Cmd {
	return tea.Batch(t.fetchAudit(), tickAudit())
}

// Update handles input, polling, and filtering.
func (t AuditTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if t.filtering {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				t.filtering = false
				t.filter.SetValue("")
				t.applyFilter()
				return t, nil
			case "enter":
				t.filtering = false
				t.applyFilter()
				return t, nil
			}
		}
		var cmd tea.Cmd
		t.filter, cmd = t.filter.Update(msg)
		return t, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		return t, nil
	case auditLoadedMsg:
		t.entries = msg.items
		t.err = msg.err
		t.applyFilter()
		return t, nil
	case FilterMsg:
		t.filter.SetValue(msg.Query)
		t.applyFilter()
		return t, nil

	case RefreshMsg:
		return t, t.fetchAudit()

	case tickMsg:
		return t, tea.Batch(t.fetchAudit(), tickAudit())
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if t.selected > 0 {
				t.selected--
			}
		case "down", "j":
			if t.selected < len(t.filtered)-1 {
				t.selected++
			}
		case "f":
			t.filtering = true
			t.filter.Focus()
			return t, nil
		}
	}
	return t, nil
}

// View renders the audit log or filter prompt.
func (t AuditTab) View() string {
	if t.filtering {
		return lipgloss.NewStyle().
			Width(t.width).
			Height(t.height).
			Padding(1).
			Render(lipgloss.JoinVertical(lipgloss.Left,
				t.theme.Title.Render("Filter audit log"),
				"",
				"Filter by actor/resource:",
				t.filter.View(),
				"",
				t.theme.Muted.Render("enter confirm · esc clear"),
			))
	}
	if t.width < 1 || t.height < 1 {
		return "Loading audit log..."
	}

	tableTheme := components.TableTheme{
		Style:    lipgloss.NewStyle(),
		Header:   lipgloss.NewStyle().Bold(true).Foreground(t.theme.TabActive.GetBackground()),
		Row:      lipgloss.NewStyle().Foreground(lipgloss.Color("#F5F1EA")),
		Selected: t.theme.RowSelected,
	}
	table := components.NewTable(tableTheme)

	headers := []string{"TIME", "ACTION", "ACTOR", "RESOURCE", "DECISION"}
	var rows []components.Row
	for _, e := range t.filtered {
		rows = append(rows, components.Row{Cells: []string{
			shortTime(e.Timestamp),
			e.Action,
			e.Actor,
			e.Resource,
			e.Decision,
		}})
	}

	body := table.View(t.width, headers, rows, t.selected)
	if t.err != nil {
		body = lipgloss.JoinVertical(lipgloss.Left, t.theme.Badge["danger"].Render(fmt.Sprintf("error: %v", t.err)), body)
	}
	return body
}

func (t *AuditTab) applyFilter() {
	q := strings.ToLower(t.filter.Value())
	if q == "" {
		t.filtered = append([]common.AuditItem(nil), t.entries...)
		return
	}
	var actorFilter, resourceFilter string
	for _, part := range strings.Fields(q) {
		if strings.HasPrefix(part, "actor:") {
			actorFilter = strings.TrimPrefix(part, "actor:")
		}
		if strings.HasPrefix(part, "resource:") {
			resourceFilter = strings.TrimPrefix(part, "resource:")
		}
	}
	t.filtered = t.filtered[:0]
	for _, e := range t.entries {
		if actorFilter != "" && !strings.Contains(strings.ToLower(e.Actor), actorFilter) {
			continue
		}
		if resourceFilter != "" && !strings.Contains(strings.ToLower(e.Resource), resourceFilter) {
			continue
		}
		t.filtered = append(t.filtered, e)
	}
}

func (t AuditTab) fetchAudit() tea.Cmd {
	return func() tea.Msg {
		items, err := t.client.ListAudit()
		return auditLoadedMsg{items: items, err: err}
	}
}

func tickAudit() tea.Cmd {
	return tickSessions()
}

type auditLoadedMsg struct {
	items []common.AuditItem
	err   error
}

func shortTime(ts string) string {
	if len(ts) > 16 {
		return ts[:16]
	}
	return ts
}
