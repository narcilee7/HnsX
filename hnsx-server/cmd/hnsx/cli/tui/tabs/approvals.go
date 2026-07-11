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

// ApprovalsTab lists pending approvals and supports approve/reject actions.
type ApprovalsTab struct {
	client *common.Client

	width  int
	height int
	theme  common.Theme

	approvals   []common.ApprovalItem
	filtered    []common.ApprovalItem
	selected    int
	filterQuery string
	err         error

	// reject input state
	rejecting bool
	input     textinput.Model
}

// NewApprovalsTab creates an approvals tab.
func NewApprovalsTab(serverURL string) ApprovalsTab {
	th := common.NewTheme()
	ti := textinput.New()
	ti.Placeholder = "reason (optional)"
	ti.Focus()
	return ApprovalsTab{
		client: common.NewClient(serverURL),
		theme:  th,
		input:  ti,
	}
}

// Init starts polling.
func (t ApprovalsTab) Init() tea.Cmd {
	return tea.Batch(t.fetchApprovals(), tickApprovals())
}

// Update handles input, polling, and actions.
func (t ApprovalsTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if t.rejecting {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				t.rejecting = false
				t.input.SetValue("")
				return t, nil
			case "enter":
				reason := t.input.Value()
				t.rejecting = false
				t.input.SetValue("")
				return t, t.reject(reason)
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
	case approvalsLoadedMsg:
		t.approvals = msg.items
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
		for i, a := range t.filtered {
			if a.ID == msg.ID {
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
		return t, t.fetchApprovals()

	case tickMsg:
		return t, tea.Batch(t.fetchApprovals(), tickApprovals())
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
		case "a":
			return t, t.approve()
		case "x":
			if t.selected >= 0 && t.selected < len(t.filtered) {
				t.rejecting = true
				t.input.Focus()
				return t, nil
			}
		}
	}
	return t, nil
}

// View renders the approvals list or the reject input prompt.
func (t ApprovalsTab) View() string {
	if t.rejecting {
		return lipgloss.NewStyle().
			Width(t.width).
			Height(t.height).
			Padding(1).
			Render(lipgloss.JoinVertical(lipgloss.Left,
				t.theme.Title.Render("Reject approval"),
				"",
				fmt.Sprintf("Approval: %s", t.approvals[t.selected].ID),
				"Enter reason (optional):",
				t.input.View(),
				"",
				t.theme.Muted.Render("enter confirm · esc cancel"),
			))
	}
	if t.width < 1 || t.height < 1 {
		return "Loading approvals..."
	}

	tableTheme := components.TableTheme{
		Style:    lipgloss.NewStyle(),
		Header:   lipgloss.NewStyle().Bold(true).Foreground(t.theme.TabActive.GetBackground()),
		Row:      lipgloss.NewStyle().Foreground(lipgloss.Color("#F5F1EA")),
		Selected: t.theme.RowSelected,
	}
	table := components.NewTable(tableTheme)

	headers := []string{"SESSION", "RISK", "REASON", "AGE"}
	var rows []components.Row
	for _, a := range t.filtered {
		rows = append(rows, components.Row{Cells: []string{
			a.SessionID,
			a.Risk,
			truncate(a.Reason, 30),
			age(a.CreatedAt),
		}})
	}

	body := table.View(t.width, headers, rows, t.selected)
	if t.err != nil {
		errLine := t.theme.Badge["danger"].Render(fmt.Sprintf("error: %v", t.err))
		body = lipgloss.JoinVertical(lipgloss.Left, errLine, body)
	}
	return body
}

func (t ApprovalsTab) fetchApprovals() tea.Cmd {
	return func() tea.Msg {
		items, err := t.client.ListApprovals()
		return approvalsLoadedMsg{items: items, err: err}
	}
}

func (t ApprovalsTab) approve() tea.Cmd {
	if t.selected < 0 || t.selected >= len(t.filtered) {
		return nil
	}
	return func() tea.Msg {
		err := t.client.ApproveApproval(t.filtered[t.selected].ID)
		return actionMsg{err: err, kind: "approve"}
	}
}

func (t ApprovalsTab) reject(reason string) tea.Cmd {
	if t.selected < 0 || t.selected >= len(t.filtered) {
		return nil
	}
	return func() tea.Msg {
		err := t.client.RejectApproval(t.filtered[t.selected].ID, reason)
		return actionMsg{err: err, kind: "reject"}
	}
}

func (t *ApprovalsTab) applyFilter() {
	if t.filterQuery == "" {
		t.filtered = append([]common.ApprovalItem(nil), t.approvals...)
		return
	}
	t.filtered = t.filtered[:0]
	for _, a := range t.approvals {
		if strings.Contains(strings.ToLower(a.ID), t.filterQuery) ||
			strings.Contains(strings.ToLower(a.SessionID), t.filterQuery) ||
			strings.Contains(strings.ToLower(a.Risk), t.filterQuery) {
			t.filtered = append(t.filtered, a)
		}
	}
}

func tickApprovals() tea.Cmd {
	return tickSessions()
}

type approvalsLoadedMsg struct {
	items []common.ApprovalItem
	err   error
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
