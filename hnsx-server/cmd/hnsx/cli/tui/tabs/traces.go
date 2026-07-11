package tabs

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/common"
	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/components"
)

// TracesTab lists traces and supports a detail view showing the raw
// observation tree.
type TracesTab struct {
	client *common.Client

	width   int
	height  int
	theme   common.Theme

	traces      []common.TraceListItem
	filtered    []common.TraceListItem
	selected    int
	filterQuery string
	err         error
	detail      *traceDetail
	inDetail    bool
}

// NewTracesTab creates a traces tab connected to the given server URL.
func NewTracesTab(serverURL string) TracesTab {
	th := common.NewTheme()
	return TracesTab{
		client: common.NewClient(serverURL),
		theme:  th,
	}
}

// Init starts the polling loop.
func (t TracesTab) Init() tea.Cmd {
	return tea.Batch(t.fetchTraces(), tickTraces())
}

// Update handles input and data messages.
func (t TracesTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		if t.detail != nil {
			t.detail.setSize(t.width, t.height)
		}
		return t, nil

	case tracesLoadedMsg:
		t.traces = msg.items
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
		for i, tr := range t.filtered {
			if tr.ID == msg.ID {
				t.selected = i
				t.inDetail = true
				t.detail = newTraceDetail(t.client, tr.ID, t.theme)
				t.detail.setSize(t.width, t.height)
				return t, t.detail.Init()
			}
		}
		for _, tr := range t.traces {
			if tr.ID == msg.ID {
				t.inDetail = true
				t.detail = newTraceDetail(t.client, tr.ID, t.theme)
				t.detail.setSize(t.width, t.height)
				return t, t.detail.Init()
			}
		}
		return t, nil

	case FilterMsg:
		t.filterQuery = strings.ToLower(msg.Query)
		t.applyFilter()
		return t, nil

	case RefreshMsg:
		return t, t.fetchTraces()

	case tickMsg:
		if !t.inDetail {
			return t, tea.Batch(t.fetchTraces(), tickTraces())
		}
		return t, tickTraces()

	case tea.KeyMsg:
		if t.inDetail && t.detail != nil {
			m, cmd := t.detail.Update(msg)
			t.detail = m.(*traceDetail)
			if t.detail.closed {
				t.inDetail = false
				t.detail = nil
			}
			return t, cmd
		}

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
			if t.selected >= 0 && t.selected < len(t.filtered) {
				t.inDetail = true
				t.detail = newTraceDetail(t.client, t.filtered[t.selected].ID, t.theme)
				t.detail.setSize(t.width, t.height)
				return t, t.detail.Init()
			}
		}
	}
	return t, nil
}

// View renders the traces list or the detail view.
func (t TracesTab) View() string {
	if t.inDetail && t.detail != nil {
		return t.detail.View()
	}
	if t.width < 1 || t.height < 1 {
		return "Loading traces..."
	}

	tableTheme := components.TableTheme{
		Style:    lipgloss.NewStyle(),
		Header:   lipgloss.NewStyle().Bold(true).Foreground(t.theme.TabActive.GetBackground()),
		Row:      lipgloss.NewStyle().Foreground(lipgloss.Color("#F5F1EA")),
		Selected: t.theme.RowSelected,
	}
	table := components.NewTable(tableTheme)

	headers := []string{"ID", "SESSION", "DOMAIN", "COST"}
	var rows []components.Row
	for _, tr := range t.filtered {
		rows = append(rows, components.Row{Cells: []string{
			tr.ID,
			tr.SessionID,
			tr.DomainID,
			fmt.Sprintf("$%.4f", tr.Cost),
		}})
	}

	body := table.View(t.width, headers, rows, t.selected)
	if t.err != nil {
		errLine := t.theme.Badge["danger"].Render(fmt.Sprintf("error: %v", t.err))
		body = lipgloss.JoinVertical(lipgloss.Left, errLine, body)
	}
	return body
}

func (t *TracesTab) applyFilter() {
	if t.filterQuery == "" {
		t.filtered = append([]common.TraceListItem(nil), t.traces...)
		return
	}
	t.filtered = t.filtered[:0]
	for _, tr := range t.traces {
		if strings.Contains(strings.ToLower(tr.ID), t.filterQuery) ||
			strings.Contains(strings.ToLower(tr.SessionID), t.filterQuery) ||
			strings.Contains(strings.ToLower(tr.DomainID), t.filterQuery) {
			t.filtered = append(t.filtered, tr)
		}
	}
}

func (t TracesTab) fetchTraces() tea.Cmd {
	return func() tea.Msg {
		items, err := t.client.ListTraces()
		return tracesLoadedMsg{items: items, err: err}
	}
}

type tracesLoadedMsg struct {
	items []common.TraceListItem
	err   error
}

func tickTraces() tea.Cmd {
	return tickSessions()
}

// traceDetail displays a trace as an indented JSON-like tree.
type traceDetail struct {
	client  *common.Client
	traceID string
	theme   common.Theme

	width  int
	height int
	lines  []string
	err    error
	closed bool
}

func newTraceDetail(client *common.Client, id string, theme common.Theme) *traceDetail {
	return &traceDetail{
		client:  client,
		traceID: id,
		theme:   theme,
	}
}

func (d *traceDetail) setSize(w, h int) {
	d.width = w
	d.height = h
}

func (d *traceDetail) Init() tea.Cmd {
	return d.fetchTrace
}

func (d *traceDetail) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc", "q":
			d.closed = true
			return d, nil
		}
	}
	if m, ok := msg.(traceDetailMsg); ok {
		d.lines = m.lines
		d.err = m.err
	}
	return d, nil
}

func (d *traceDetail) View() string {
	if d.width < 1 {
		return "Loading trace..."
	}
	header := d.theme.Header.Render(fmt.Sprintf("Trace %s  │  esc/q back", d.traceID))
	content := strings.Join(d.lines, "\n")
	if content == "" {
		content = d.theme.Muted.Render("No observations")
	}
	if d.err != nil {
		content = lipgloss.JoinVertical(lipgloss.Left, content, "", d.theme.Badge["danger"].Render(fmt.Sprintf("error: %v", d.err)))
	}
	return lipgloss.JoinVertical(lipgloss.Top, header, content)
}

func (d *traceDetail) fetchTrace() tea.Msg {
	tree, err := d.client.GetTrace(d.traceID)
	if err != nil {
		return traceDetailMsg{err: err}
	}
	return traceDetailMsg{lines: renderTree(tree, "", 0)}
}

type traceDetailMsg struct {
	lines []string
	err   error
}

func renderTree(node map[string]any, prefix string, depth int) []string {
	var lines []string
	indent := strings.Repeat("  ", depth)
	for k, v := range node {
		switch val := v.(type) {
		case map[string]any:
			lines = append(lines, fmt.Sprintf("%s%s%s:", prefix, indent, k))
			lines = append(lines, renderTree(val, prefix, depth+1)...)
		case []any:
			lines = append(lines, fmt.Sprintf("%s%s%s: [%d]", prefix, indent, k, len(val)))
			for i, item := range val {
				if m, ok := item.(map[string]any); ok {
					lines = append(lines, fmt.Sprintf("%s%s  [%d]", prefix, indent, i))
					lines = append(lines, renderTree(m, prefix, depth+2)...)
				} else {
					lines = append(lines, fmt.Sprintf("%s%s  [%d] %v", prefix, indent, i, item))
				}
			}
		default:
			lines = append(lines, fmt.Sprintf("%s%s%s: %v", prefix, indent, k, val))
		}
	}
	return lines
}
