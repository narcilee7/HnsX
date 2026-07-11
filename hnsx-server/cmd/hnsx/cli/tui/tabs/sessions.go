package tabs

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/common"
	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/components"
	"github.com/hnsx-io/hnsx/server/internal/client"
)

// SessionsTab lists sessions and supports a detail/tail view via SSE.
type SessionsTab struct {
	client *common.Client

	width  int
	height int
	theme  common.Theme

	sessions    []client.SessionListItem
	filtered    []client.SessionListItem
	selected    int
	filterQuery string
	err         error
	detail      *sessionDetail

	// view toggles
	inDetail bool
}

// NewSessionsTab creates a sessions tab connected to the given server URL.
func NewSessionsTab(serverURL string) SessionsTab {
	th := common.NewTheme()
	return SessionsTab{
		client: common.NewClient(serverURL),
		theme:  th,
	}
}

// Init starts the polling loop.
func (t SessionsTab) Init() tea.Cmd {
	return tea.Batch(t.fetchSessions(), tickSessions())
}

// Update handles input and data messages.
func (t SessionsTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		if t.detail != nil {
			t.detail.setSize(t.width, t.detailHeight())
		}
		return t, nil

	case sessionsLoadedMsg:
		t.sessions = msg.items
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
				t.inDetail = true
				t.detail = newSessionDetail(t.client, s.ID, t.theme)
				t.detail.setSize(t.width, t.detailHeight())
				return t, t.detail.Init()
			}
		}
		// If not found in current filtered list, try full list and open directly.
		for _, s := range t.sessions {
			if s.ID == msg.ID {
				t.inDetail = true
				t.detail = newSessionDetail(t.client, s.ID, t.theme)
				t.detail.setSize(t.width, t.detailHeight())
				return t, t.detail.Init()
			}
		}
		return t, nil

	case FilterMsg:
		t.filterQuery = strings.ToLower(msg.Query)
		t.applyFilter()
		return t, nil

	case RefreshMsg:
		if !t.inDetail {
			return t, tea.Batch(t.fetchSessions(), tickSessions())
		}
		return t, tickSessions()

	case tea.KeyMsg:
		if t.inDetail && t.detail != nil {
			m, cmd := t.detail.Update(msg)
			t.detail = m.(*sessionDetail)
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
				t.detail = newSessionDetail(t.client, t.filtered[t.selected].ID, t.theme)
				t.detail.setSize(t.width, t.detailHeight())
				return t, t.detail.Init()
			}
		case "r":
			return t, t.rerun()
		case "x":
			return t, t.cancel()
		}
	}
	return t, nil
}

// View renders the sessions list or the detail view.
func (t SessionsTab) View() string {
	if t.inDetail && t.detail != nil {
		return t.detail.View()
	}
	if t.width < 1 || t.height < 1 {
		return "Loading sessions..."
	}

	tableTheme := components.TableTheme{
		Style:    lipgloss.NewStyle(),
		Header:   lipgloss.NewStyle().Bold(true).Foreground(t.theme.TabActive.GetBackground()),
		Row:      lipgloss.NewStyle().Foreground(lipgloss.Color("#F5F1EA")),
		Selected: t.theme.RowSelected,
	}
	table := components.NewTable(tableTheme)

	headers := []string{"STATE", "DOMAIN", "ID", "AGE"}
	var rows []components.Row
	for _, s := range t.filtered {
		rows = append(rows, components.Row{Cells: []string{
			s.State,
			s.DomainID,
			s.ID,
			age(s.StartedAt),
		}})
	}

	body := table.View(t.width, headers, rows, t.selected)
	if t.err != nil {
		errLine := t.theme.Badge["danger"].Render(fmt.Sprintf("error: %v", t.err))
		body = lipgloss.JoinVertical(lipgloss.Left, errLine, body)
	}
	return body
}

func (t SessionsTab) detailHeight() int {
	// Reserve one line for header if needed; detail uses remaining height.
	return t.height
}

func (t SessionsTab) fetchSessions() tea.Cmd {
	return func() tea.Msg {
		items, err := t.client.ListSessions()
		return sessionsLoadedMsg{items: items, err: err}
	}
}

func (t SessionsTab) rerun() tea.Cmd {
	if t.selected < 0 || t.selected >= len(t.filtered) {
		return nil
	}
	return func() tea.Msg {
		_, err := t.client.RerunSession(t.filtered[t.selected].ID)
		return actionMsg{err: err, kind: "rerun"}
	}
}

func (t SessionsTab) cancel() tea.Cmd {
	if t.selected < 0 || t.selected >= len(t.filtered) {
		return nil
	}
	return func() tea.Msg {
		_, err := t.client.CancelSession(t.filtered[t.selected].ID)
		return actionMsg{err: err, kind: "cancel"}
	}
}

func (t *SessionsTab) applyFilter() {
	if t.filterQuery == "" {
		t.filtered = append([]client.SessionListItem(nil), t.sessions...)
		return
	}
	t.filtered = t.filtered[:0]
	for _, s := range t.sessions {
		if strings.Contains(strings.ToLower(s.State), t.filterQuery) ||
			strings.Contains(strings.ToLower(s.DomainID), t.filterQuery) ||
			strings.Contains(strings.ToLower(s.ID), t.filterQuery) {
			t.filtered = append(t.filtered, s)
		}
	}
}

func tickSessions() tea.Cmd {
	return tea.Tick(2*time.Second, func(_ time.Time) tea.Msg {
		return tickMsg{}
	})
}

type tickMsg struct{}
type sessionsLoadedMsg struct {
	items []client.SessionListItem
	err   error
}
type actionMsg struct {
	err  error
	kind string
}

func age(started string) string {
	if started == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, started)
	if err != nil {
		return started
	}
	d := time.Since(t)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

// sessionDetail is the SSE tail view for a single session.
type sessionDetail struct {
	client    *common.Client
	sessionID string
	theme     common.Theme

	width  int
	height int
	lines  []string
	err    error
	closed bool

	ctx    context.Context
	cancel context.CancelFunc
	events chan client.Event
	errCh  chan error
}

func newSessionDetail(client *common.Client, id string, theme common.Theme) *sessionDetail {
	return &sessionDetail{
		client:    client,
		sessionID: id,
		theme:     theme,
	}
}

func (d *sessionDetail) setSize(w, h int) {
	d.width = w
	d.height = h
}

func (d *sessionDetail) Init() tea.Cmd {
	d.ctx, d.cancel = context.WithCancel(context.Background())
	d.events = make(chan client.Event)
	d.errCh = make(chan error, 1)
	go d.readEvents()
	return tea.Batch(d.fetchSession(), d.waitEvent())
}

func (d *sessionDetail) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			d.closed = true
			if d.cancel != nil {
				d.cancel()
			}
			return d, nil
		}
	case sessionDetailMsg:
		if msg.err != nil {
			d.err = msg.err
		}
		if msg.line != "" {
			d.lines = append(d.lines, msg.line)
			if len(d.lines) > 500 {
				d.lines = d.lines[len(d.lines)-500:]
			}
		}
		if msg.done {
			return d, nil
		}
		return d, d.waitEvent()
	}
	return d, nil
}

func (d *sessionDetail) View() string {
	if d.width < 1 {
		return "Loading session..."
	}
	header := d.theme.Header.Render(fmt.Sprintf("Session %s  │  esc/q back", d.sessionID))
	content := strings.Join(d.lines, "\n")
	if content == "" {
		content = d.theme.Muted.Render("Waiting for events...")
	}
	if d.err != nil {
		content = lipgloss.JoinVertical(lipgloss.Left, content, "", d.theme.Badge["danger"].Render(fmt.Sprintf("error: %v", d.err)))
	}
	return lipgloss.JoinVertical(lipgloss.Top, header, content)
}

func (d *sessionDetail) fetchSession() tea.Cmd {
	return func() tea.Msg {
		s, err := d.client.GetSession(d.sessionID)
		if err != nil {
			return sessionDetailMsg{err: err}
		}
		return sessionDetailMsg{line: fmt.Sprintf("state: %s  domain: %s  version: %s", s.State, s.DomainID, s.DomainVersion)}
	}
}

// readEvents runs in a background goroutine and forwards SSE events into the
// model's event channel. It exits when the context is cancelled or the stream
// ends.
func (d *sessionDetail) readEvents() {
	defer close(d.events)
	defer close(d.errCh)

	events, errCh, err := d.client.StreamSessionEvents(d.ctx, d.sessionID)
	if err != nil {
		select {
		case d.errCh <- err:
		case <-d.ctx.Done():
		}
		return
	}
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			select {
			case d.events <- ev:
			case <-d.ctx.Done():
				return
			}
		case err := <-errCh:
			select {
			case d.errCh <- err:
			case <-d.ctx.Done():
			}
			return
		case <-d.ctx.Done():
			return
		}
	}
}

// waitEvent blocks until the next SSE event or error arrives. It is returned
// as a tea.Cmd so the runtime keeps scheduling it after each event.
func (d *sessionDetail) waitEvent() tea.Cmd {
	return func() tea.Msg {
		select {
		case ev, ok := <-d.events:
			if !ok {
				return sessionDetailMsg{done: true}
			}
			return sessionDetailMsg{line: fmt.Sprintf("[%s] %s", ev.Name, string(ev.Payload))}
		case err := <-d.errCh:
			if err != nil {
				return sessionDetailMsg{err: err, done: true}
			}
			return sessionDetailMsg{done: true}
		case <-d.ctx.Done():
			return sessionDetailMsg{done: true}
		}
	}
}

type sessionDetailMsg struct {
	line string
	err  error
	done bool
}
