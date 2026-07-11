package tabs

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/common"
	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/components"
	"github.com/hnsx-io/hnsx/server/internal/client"
)

// EvalTab lists eval sets and supports run detail / diff.
type EvalTab struct {
	client *common.Client

	width  int
	height int
	theme  common.Theme

	sets        []client.EvalSet
	filtered    []client.EvalSet
	selected    int
	filterQuery string
	err         error

	// detail state
	inDetail bool
	detail   *evalDetail
}

// NewEvalTab creates an eval tab.
func NewEvalTab(serverURL string) EvalTab {
	th := common.NewTheme()
	return EvalTab{
		client: common.NewClient(serverURL),
		theme:  th,
	}
}

// Init starts polling.
func (t EvalTab) Init() tea.Cmd {
	return t.fetchSets()
}

// Update handles input and data.
func (t EvalTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if t.inDetail && t.detail != nil {
		m, cmd := t.detail.Update(msg)
		t.detail = m.(*evalDetail)
		if t.detail.closed {
			t.inDetail = false
			t.detail = nil
		}
		return t, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		return t, nil
	case evalSetsLoadedMsg:
		t.sets = msg.items
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
		for i, s := range t.filtered {
			if s.ID == msg.ID {
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
		return t, t.fetchSets()

	case tickMsg:
		return t, t.fetchSets()
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
			if t.selected >= 0 && t.selected < len(t.filtered) {
				t.inDetail = true
				t.detail = newEvalDetail(t.client, t.filtered[t.selected].ID, t.theme)
				t.detail.setSize(t.width, t.height)
				return t, t.detail.Init()
			}
		case "r":
			return t, t.runEval()
		}
	}
	return t, nil
}

// View renders the eval set list or detail view.
func (t EvalTab) View() string {
	if t.inDetail && t.detail != nil {
		return t.detail.View()
	}
	if t.width < 1 || t.height < 1 {
		return "Loading eval sets..."
	}

	tableTheme := components.TableTheme{
		Style:    lipgloss.NewStyle(),
		Header:   lipgloss.NewStyle().Bold(true).Foreground(t.theme.TabActive.GetBackground()),
		Row:      lipgloss.NewStyle().Foreground(lipgloss.Color("#F5F1EA")),
		Selected: t.theme.RowSelected,
	}
	table := components.NewTable(tableTheme)

	headers := []string{"ID", "DOMAIN", "CASES", "LAST RUN"}
	var rows []components.Row
	for _, s := range t.filtered {
		rows = append(rows, components.Row{Cells: []string{
			s.ID,
			s.DomainID,
			fmt.Sprintf("%d", s.CaseCount),
			age(s.UpdatedAt),
		}})
	}

	body := table.View(t.width, headers, rows, t.selected)
	if t.err != nil {
		errLine := t.theme.Badge["danger"].Render(fmt.Sprintf("error: %v", t.err))
		body = lipgloss.JoinVertical(lipgloss.Left, errLine, body)
	}
	return body
}

func (t EvalTab) fetchSets() tea.Cmd {
	return func() tea.Msg {
		items, err := t.client.ListEvalSets()
		return evalSetsLoadedMsg{items: items, err: err}
	}
}

func (t EvalTab) runEval() tea.Cmd {
	if t.selected < 0 || t.selected >= len(t.filtered) {
		return nil
	}
	return func() tea.Msg {
		_, err := t.client.RunEval(t.filtered[t.selected].ID)
		return actionMsg{err: err, kind: "run-eval"}
	}
}

func (t *EvalTab) applyFilter() {
	if t.filterQuery == "" {
		t.filtered = append([]client.EvalSet(nil), t.sets...)
		return
	}
	t.filtered = t.filtered[:0]
	for _, s := range t.sets {
		if strings.Contains(strings.ToLower(s.ID), t.filterQuery) ||
			strings.Contains(strings.ToLower(s.DomainID), t.filterQuery) {
			t.filtered = append(t.filtered, s)
		}
	}
}

type evalSetsLoadedMsg struct {
	items []client.EvalSet
	err   error
}

// evalDetail shows runs for a single eval set and supports run + diff.
type evalDetail struct {
	client *common.Client
	setID  string
	theme  common.Theme

	width    int
	height   int
	runs     []client.EvalRun
	selected int
	err      error
	closed   bool
}

func newEvalDetail(client *common.Client, setID string, theme common.Theme) *evalDetail {
	return &evalDetail{
		client: client,
		setID:  setID,
		theme:  theme,
	}
}

func (d *evalDetail) setSize(w, h int) {
	d.width = w
	d.height = h
}

func (d *evalDetail) Init() tea.Cmd {
	return d.fetchRuns
}

func (d *evalDetail) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			d.closed = true
			return d, nil
		case "up", "k":
			if d.selected > 0 {
				d.selected--
			}
		case "down", "j":
			if d.selected < len(d.runs)-1 {
				d.selected++
			}
		case "r":
			return d, d.runEval()
		case "d":
			return d, d.diff()
		}
	case evalRunsLoadedMsg:
		d.runs = msg.items
		d.err = msg.err
		if d.selected >= len(d.runs) {
			d.selected = len(d.runs) - 1
		}
		if d.selected < 0 {
			d.selected = 0
		}
	case actionMsg:
		if msg.err != nil {
			d.err = msg.err
		}
	}
	return d, nil
}

func (d *evalDetail) View() string {
	if d.width < 1 {
		return "Loading eval runs..."
	}
	header := d.theme.Header.Render(fmt.Sprintf("Eval set %s  │  esc/q back · r run · d diff", d.setID))

	tableTheme := components.TableTheme{
		Style:    lipgloss.NewStyle(),
		Header:   lipgloss.NewStyle().Bold(true).Foreground(d.theme.TabActive.GetBackground()),
		Row:      lipgloss.NewStyle().Foreground(lipgloss.Color("#F5F1EA")),
		Selected: d.theme.RowSelected,
	}
	table := components.NewTable(tableTheme)

	headers := []string{"RUN", "STATE", "SCORE", "COST", "AGE"}
	var rows []components.Row
	for _, r := range d.runs {
		rows = append(rows, components.Row{Cells: []string{
			r.ID,
			r.State,
			fmt.Sprintf("%.2f", r.Score),
			fmt.Sprintf("$%.4f", r.TotalCostUSD),
			age(r.CreatedAt),
		}})
	}

	body := table.View(d.width, headers, rows, d.selected)
	if d.err != nil {
		body = lipgloss.JoinVertical(lipgloss.Left, body, "", d.theme.Badge["danger"].Render(fmt.Sprintf("error: %v", d.err)))
	}
	return lipgloss.JoinVertical(lipgloss.Top, header, body)
}

func (d *evalDetail) fetchRuns() tea.Msg {
	items, err := d.client.ListEvalRuns(d.setID)
	return evalRunsLoadedMsg{items: items, err: err}
}

func (d *evalDetail) runEval() tea.Cmd {
	return func() tea.Msg {
		_, err := d.client.RunEval(d.setID)
		return actionMsg{err: err, kind: "run-eval"}
	}
}

func (d *evalDetail) diff() tea.Cmd {
	return func() tea.Msg {
		if len(d.runs) < 2 {
			return actionMsg{err: fmt.Errorf("need at least 2 runs to diff"), kind: "diff"}
		}
		// Sort by creation time descending (newest first) and diff the top two.
		runs := append([]client.EvalRun(nil), d.runs...)
		sort.Slice(runs, func(i, j int) bool {
			return runs[i].CreatedAt > runs[j].CreatedAt
		})
		a, b := runs[0], runs[1]
		return actionMsg{err: fmt.Errorf("diff %s vs %s not yet implemented in TUI", a.ID, b.ID), kind: "diff"}
	}
}

type evalRunsLoadedMsg struct {
	items []client.EvalRun
	err   error
}
