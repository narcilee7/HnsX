package tabs

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/common"
	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/components"
	"github.com/hnsx-io/hnsx/server/internal/client"
)

// DomainsTab lists domains and supports spec view + trigger.
type DomainsTab struct {
	client *common.Client

	width  int
	height int
	theme  common.Theme

	domains     []client.DomainListItem
	filtered    []client.DomainListItem
	selected    int
	filterQuery string
	err         error

	// trigger state
	triggering bool
	input      textinput.Model
}

// NewDomainsTab creates a domains tab.
func NewDomainsTab(serverURL string) DomainsTab {
	th := common.NewTheme()
	ti := textinput.New()
	ti.Placeholder = `{}`
	return DomainsTab{
		client: common.NewClient(serverURL),
		theme:  th,
		input:  ti,
	}
}

// Init starts polling.
func (t DomainsTab) Init() tea.Cmd {
	return tea.Batch(t.fetchDomains(), tickDomains())
}

// Update handles input and polling.
func (t DomainsTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if t.triggering {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				t.triggering = false
				t.input.SetValue("")
				return t, nil
			case "enter":
				payload := t.input.Value()
				t.triggering = false
				t.input.SetValue("")
				return t, t.trigger(payload)
			}
		}
		var cmd tea.Cmd
		t.input, cmd = t.input.Update(msg)
		return t, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		return t, nil
	case domainsLoadedMsg:
		t.domains = msg.items
		t.err = msg.err
		t.applyFilter()
		if t.selected >= len(t.filtered) {
			t.selected = len(t.filtered) - 1
		}
		if t.selected < 0 {
			t.selected = 0
		}
		return t, nil

	case SelectMsg:
		for i, d := range t.filtered {
			if d.ID == msg.ID {
				t.selected = i
				return t, nil
			}
		}
		return t, nil

	case FilterMsg:
		t.filterQuery = strings.ToLower(msg.Query)
		t.applyFilter()
		return t, nil

	case RefreshMsg:
		return t, t.fetchDomains()

	case tickMsg:
		return t, tea.Batch(t.fetchDomains(), tickDomains())
	case actionMsg:
		if msg.err != nil {
			t.err = msg.err
		}
		return t, nil
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
		case "enter":
			// TODO: show domain spec detail view in a follow-up polish phase.
		case "R":
			if t.selected >= 0 && t.selected < len(t.filtered) {
				t.triggering = true
				t.input.Focus()
				return t, nil
			}
		}
	}
	return t, nil
}

// View renders the domain list or trigger prompt.
func (t DomainsTab) View() string {
	if t.triggering {
		return lipgloss.NewStyle().
			Width(t.width).
			Height(t.height).
			Padding(1).
			Render(lipgloss.JoinVertical(lipgloss.Left,
				t.theme.Title.Render("Trigger session"),
				"",
				fmt.Sprintf("Domain: %s", t.domains[t.selected].ID),
				"Enter trigger JSON:",
				t.input.View(),
				"",
				t.theme.Muted.Render("enter confirm · esc cancel"),
			))
	}
	if t.width < 1 || t.height < 1 {
		return "Loading domains..."
	}

	tableTheme := components.TableTheme{
		Style:    lipgloss.NewStyle(),
		Header:   lipgloss.NewStyle().Bold(true).Foreground(t.theme.TabActive.GetBackground()),
		Row:      lipgloss.NewStyle().Foreground(lipgloss.Color("#F5F1EA")),
		Selected: t.theme.RowSelected,
	}
	table := components.NewTable(tableTheme)

	headers := []string{"ID", "VERSION", "STATUS", "UPDATED"}
	var rows []components.Row
	for _, d := range t.filtered {
		rows = append(rows, components.Row{Cells: []string{
			d.ID,
			d.Version,
			d.Status,
			age(d.UpdatedAt),
		}})
	}

	body := table.View(t.width, headers, rows, t.selected)
	if t.err != nil {
		body = lipgloss.JoinVertical(lipgloss.Left, t.theme.Badge["danger"].Render(fmt.Sprintf("error: %v", t.err)), body)
	}
	return body
}

func (t *DomainsTab) applyFilter() {
	if t.filterQuery == "" {
		t.filtered = append([]client.DomainListItem(nil), t.domains...)
		return
	}
	t.filtered = t.filtered[:0]
	for _, d := range t.domains {
		if strings.Contains(strings.ToLower(d.ID), t.filterQuery) ||
			strings.Contains(strings.ToLower(d.Version), t.filterQuery) ||
			strings.Contains(strings.ToLower(d.Status), t.filterQuery) {
			t.filtered = append(t.filtered, d)
		}
	}
}

func (t DomainsTab) fetchDomains() tea.Cmd {
	return func() tea.Msg {
		items, err := t.client.ListDomains()
		return domainsLoadedMsg{items: items, err: err}
	}
}

func (t DomainsTab) trigger(payload string) tea.Cmd {
	if t.selected < 0 || t.selected >= len(t.filtered) {
		return nil
	}
	trigger := map[string]any{}
	if payload != "" {
		if err := json.Unmarshal([]byte(payload), &trigger); err != nil {
			return func() tea.Msg { return actionMsg{err: err, kind: "trigger"} }
		}
	}
	return func() tea.Msg {
		_, err := t.client.TriggerSession(t.filtered[t.selected].ID, trigger)
		return actionMsg{err: err, kind: "trigger"}
	}
}

func tickDomains() tea.Cmd {
	return tickSessions()
}

type domainsLoadedMsg struct {
	items []client.DomainListItem
	err   error
}
