package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/common"
	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/components"
	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/tabs"
)

// tabName is the ordered list of tabs shown in the tab bar.
var tabNames = []string{
	"Sessions",
	"Traces",
	"Approvals",
	"Eval",
	"Audit",
	"Domains",
	"Dashboard",
}

// Model is the root bubbletea model for the HnsX TUI.
type Model struct {
	serverURL string
	client    *common.Client
	width     int
	height    int
	theme     common.Theme
	keys      KeyMap
	statusBar StatusBar
	help      components.Help

	activeTab int
	helpOpen  bool
	serverOK  bool
	tabs      []tea.Model

	// command mode state
	commandMode   bool
	commandInput  textinput.Model
	commandList   components.CommandList
	commandResult string
	commandErr    error
}

// NewModel creates the root TUI model.
func NewModel(serverURL string) Model {
	th := common.NewTheme()
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = "session id, approve id, quit ..."
	return Model{
		serverURL: serverURL,
		client:    common.NewClient(serverURL),
		theme:     th,
		keys:      DefaultKeyMap(),
		statusBar: NewStatusBar(th),
		help:      components.NewHelp(th.Help),
		serverOK:  true, // optimistic until first health check
		commandInput: ti,
		commandList:  components.NewCommandList(th),
		tabs: []tea.Model{
			tabs.NewSessionsTab(serverURL),
			tabs.NewTracesTab(serverURL),
			tabs.NewApprovalsTab(serverURL),
			tabs.NewEvalTab(serverURL),
			tabs.NewAuditTab(serverURL),
			tabs.NewDomainsTab(serverURL),
			tabs.NewDashboardTab(serverURL),
		},
	}
}

// Init starts background ticks and child models.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		tickCmd(),
		healthCheck(m.client),
	}
	for _, t := range m.tabs {
		cmds = append(cmds, t.Init())
	}
	return tea.Batch(cmds...)
}

// Update handles global input and delegates to the active tab.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Help overlay takes precedence.
	if m.helpOpen {
		if k, ok := msg.(tea.KeyMsg); ok {
			if keyMatches(k, m.keys.Back, m.keys.Help, m.keys.Quit) {
				m.helpOpen = false
				return m, nil
			}
		}
		return m, nil
	}

	// Command mode input handling.
	if m.commandMode {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.commandMode = false
				m.commandInput.SetValue("")
				m.commandInput.Blur()
				m.commandResult = ""
				m.commandErr = nil
				return m, nil
			case "enter":
				return m.acceptCommand()
			case "up":
				m.commandList.MoveUp()
				return m, nil
			case "down":
				m.commandList.MoveDown()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.commandInput, cmd = m.commandInput.Update(msg)
		m.commandList.SetInput(m.commandInput.Value())
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case keyMatches(msg, m.keys.Quit):
			return m, tea.Quit
		case keyMatches(msg, m.keys.Help):
			m.helpOpen = true
			return m, nil
		case keyMatches(msg, m.keys.NextTab):
			m.activeTab = (m.activeTab + 1) % len(tabNames)
			return m, nil
		case keyMatches(msg, m.keys.PrevTab):
			m.activeTab = (m.activeTab - 1 + len(tabNames)) % len(tabNames)
			return m, nil
		case msg.String() == "/":
			m.commandMode = true
			m.commandInput.SetValue("/")
			m.commandInput.Focus()
			m.commandList.SetInput("/")
			return m, textinput.Blink
		}

		// Number keys 1-7 switch tabs directly.
		if n := tabNumber(msg.String()); n > 0 && n <= len(tabNames) {
			m.activeTab = n - 1
			return m, nil
		}

		// Unhandled keys are delegated to the active tab (e.g. j/k/up/down).
		var cmd tea.Cmd
		m.tabs[m.activeTab], cmd = m.tabs[m.activeTab].Update(msg)
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		for i := range m.tabs {
			m.tabs[i], _ = m.tabs[i].Update(msg)
		}
		return m, nil

	case tickMsg:
		return m, tea.Batch(tickCmd(), healthCheck(m.client))

	case healthMsg:
		m.serverOK = msg.ok

	case tabs.CommandResultMsg:
		m.commandResult = msg.Info
		m.commandErr = msg.Err

	// Let active tab handle its own messages.
	default:
		var cmd tea.Cmd
		m.tabs[m.activeTab], cmd = m.tabs[m.activeTab].Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders the full TUI layout.
func (m Model) View() string {
	if m.width < 1 || m.height < 1 {
		return "Initializing..."
	}

	// Header
	header := m.statusBar.View(m.width, m.serverURL, m.serverOK)
	headerHeight := lipgloss.Height(header)

	// Tab bar
	tabBar := m.renderTabBar()
	tabBarHeight := lipgloss.Height(tabBar)

	// Footer / command bar
	footer := m.theme.Footer.Render(m.renderFooter())
	footerHeight := lipgloss.Height(footer)

	// Command palette appears above the footer while selecting a command.
	paletteHeight := 0
	var palette string
	if m.commandMode {
		m.commandList.SetWidth(m.width)
		palette = m.commandList.View()
		paletteHeight = lipgloss.Height(palette)
	}

	// Body height accounts for header + tab bar + footer + optional palette.
	bodyHeight := m.height - headerHeight - tabBarHeight - footerHeight - paletteHeight
	if bodyHeight < 0 {
		bodyHeight = 0
	}

	// Resize active tab view to body area.
	bodyMsg := tea.WindowSizeMsg{Width: m.width, Height: bodyHeight}
	active := m.tabs[m.activeTab]
	active, _ = active.Update(bodyMsg)
	body := active.View()

	// Stack vertically.
	content := lipgloss.JoinVertical(lipgloss.Left, header, tabBar, body, palette, footer)

	if m.helpOpen {
		helpLines := m.helpLines()
		overlay := m.help.View(m.width, m.height, helpLines)
		return overlay
	}
	return content
}

func (m Model) renderTabBar() string {
	var parts []string
	for i, name := range tabNames {
		style := m.theme.TabInactive
		if i == m.activeTab {
			style = m.theme.TabActive
		}
		parts = append(parts, style.Render(name))
	}
	return lipgloss.NewStyle().
		Width(m.width).
		Padding(0, 1).
		Render(lipgloss.JoinHorizontal(lipgloss.Top, parts...))
}

func (m Model) renderFooter() string {
	if m.commandMode {
		return m.commandInput.View()
	}
	extra := ""
	if m.commandErr != nil {
		extra = m.theme.Badge["danger"].Render(fmt.Sprintf("  %v", m.commandErr))
	} else if m.commandResult != "" {
		extra = m.theme.Badge["success"].Render(fmt.Sprintf("  %s", m.commandResult))
	}
	return fmt.Sprintf("tab:%d/%d  │  ↑↓/jk 选择  │  enter 详情  │  ? help  │  / command  │  q quit%s", m.activeTab+1, len(tabNames), extra)
}

func (m Model) helpLines() []string {
	return []string{
		"HnsX TUI 快捷键",
		"",
		"1-7      切换 tab",
		"tab      下一个 tab",
		"shift+tab  上一个 tab",
		"?        显示帮助",
		"q        退出",
		"/        命令模式（/session <id>、/approve <id> 等）",
		"esc      返回",
		"r        刷新",
	}
}

// tabNumber maps "1".."7" to their numeric value; other strings return 0.
func tabNumber(s string) int {
	if len(s) != 1 {
		return 0
	}
	c := s[0]
	if c >= '1' && c <= '7' {
		return int(c - '0')
	}
	return 0
}

func keyMatches(msg tea.KeyMsg, targets ...key.Binding) bool {
	for _, kb := range targets {
		if kb.Enabled() && kb.Help().Key == msg.String() {
			return true
		}
		for _, k := range kb.Keys() {
			if k == msg.String() {
				return true
			}
		}
	}
	return false
}

// placeholderTab is a minimal tab used until the real implementation lands.
type placeholderTab struct {
	name   string
	width  int
	height int
}

func newPlaceholderTab(name string) placeholderTab {
	return placeholderTab{name: name}
}

func (p placeholderTab) Init() tea.Cmd { return nil }

func (p placeholderTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m, ok := msg.(tea.WindowSizeMsg); ok {
		p.width = m.Width
		p.height = m.Height
	}
	return p, nil
}

func (p placeholderTab) View() string {
	if p.width < 1 {
		return fmt.Sprintf("%s tab (placeholder)", p.name)
	}
	line := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9C9C92")).
		Render(fmt.Sprintf("%s tab — coming in a later phase", p.name))
	return lipgloss.NewStyle().
		Width(p.width).
		Height(p.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render(line)
}

// tickMsg triggers periodic UI refreshes (clock, pending counts, etc.).
type tickMsg struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(_ time.Time) tea.Msg {
		return tickMsg{}
	})
}

// healthMsg carries the latest server health status.
type healthMsg struct {
	ok bool
}

func healthCheck(c *common.Client) tea.Cmd {
	return func() tea.Msg {
		return healthMsg{ok: c.Health()}
	}
}

var tickInterval = 2 * time.Second

// SetTickInterval is used by tests to speed up or slow down ticks.
func SetTickInterval(d time.Duration) {
	tickInterval = d
}

// command represents a parsed /command input.
type command struct {
	name   string
	args   []string
	kwargs map[string]string
}

// parseCommand parses "/name arg1 arg2 key=value" into a command struct.
// The leading slash is optional.
func parseCommand(input string) command {
	input = strings.TrimSpace(input)
	input = strings.TrimPrefix(input, "/")
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return command{}
	}
	c := command{
		name:   strings.ToLower(parts[0]),
		kwargs: map[string]string{},
	}
	for _, p := range parts[1:] {
		if idx := strings.Index(p, "="); idx > 0 {
			c.kwargs[p[:idx]] = p[idx+1:]
		} else {
			c.args = append(c.args, p)
		}
	}
	return c
}

// acceptCommand either selects a command from the palette or executes the current input.
func (m Model) acceptCommand() (tea.Model, tea.Cmd) {
	// If the palette is visible, accept the selected command and fill the input.
	if m.commandList.Visible() {
		if cmd, ok := m.commandList.Selected(); ok {
			if cmd.NoArgs {
				m.commandInput.SetValue("/" + cmd.Name)
				return m.dispatchCommand(m.commandInput.Value())
			}
			m.commandInput.SetValue("/" + cmd.Name + " ")
			m.commandList.SetInput(m.commandInput.Value())
			return m, textinput.Blink
		}
	}
	return m.dispatchCommand(m.commandInput.Value())
}

// dispatchCommand executes a parsed command and returns the updated model + cmd.
func (m Model) dispatchCommand(input string) (tea.Model, tea.Cmd) {
	m.commandMode = false
	m.commandInput.SetValue("")
	m.commandInput.Blur()
	m.commandResult = ""
	m.commandErr = nil

	cmd := parseCommand(input)
	switch cmd.name {
	case "", "help":
		m.commandResult = "commands: session, trace, domain, approve, reject, trigger, filter, refresh, quit"
	case "quit", "q":
		return m, tea.Quit
	case "session", "s":
		if len(cmd.args) == 0 {
			m.commandErr = fmt.Errorf("usage: /session <id>")
			return m, nil
		}
		m.activeTab = 0
		return m.sendToTab(0, tabs.SelectMsg{ID: cmd.args[0]})
	case "trace", "t":
		if len(cmd.args) == 0 {
			m.commandErr = fmt.Errorf("usage: /trace <id>")
			return m, nil
		}
		m.activeTab = 1
		return m.sendToTab(1, tabs.SelectMsg{ID: cmd.args[0]})
	case "domain", "d":
		if len(cmd.args) == 0 {
			m.commandErr = fmt.Errorf("usage: /domain <id>")
			return m, nil
		}
		m.activeTab = 5
		return m.sendToTab(5, tabs.SelectMsg{ID: cmd.args[0]})
	case "approve", "a":
		if len(cmd.args) == 0 {
			m.commandErr = fmt.Errorf("usage: /approve <id>")
			return m, nil
		}
		return m, m.approve(cmd.args[0])
	case "reject", "r":
		if len(cmd.args) == 0 {
			m.commandErr = fmt.Errorf("usage: /reject <id> [reason]")
			return m, nil
		}
		reason := ""
		if len(cmd.args) > 1 {
			reason = strings.Join(cmd.args[1:], " ")
		}
		return m, m.reject(cmd.args[0], reason)
	case "trigger":
		if len(cmd.args) == 0 {
			m.commandErr = fmt.Errorf("usage: /trigger <domain> [json]")
			return m, nil
		}
		trigger, err := parseTriggerJSON(strings.Join(cmd.args[1:], " "))
		if err != nil {
			m.commandErr = fmt.Errorf("invalid trigger: %w", err)
			return m, nil
		}
		return m, m.trigger(cmd.args[0], trigger)
	case "filter", "f":
		query := ""
		if len(cmd.args) > 0 {
			query = strings.Join(cmd.args, " ")
		}
		return m.sendToTab(m.activeTab, tabs.FilterMsg{Query: query})
	case "refresh", "re":
		return m.sendToTab(m.activeTab, tabs.RefreshMsg{})
	default:
		m.commandErr = fmt.Errorf("unknown command: %s", cmd.name)
	}
	return m, nil
}

func (m Model) sendToTab(idx int, msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.tabs[idx], cmd = m.tabs[idx].Update(msg)
	return m, cmd
}

func (m Model) approve(id string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.ApproveApproval(id)
		if err != nil {
			return tabs.CommandResultMsg{Err: err}
		}
		return tabs.CommandResultMsg{Info: fmt.Sprintf("approved %s", id)}
	}
}

func (m Model) reject(id, reason string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.RejectApproval(id, reason)
		if err != nil {
			return tabs.CommandResultMsg{Err: err}
		}
		return tabs.CommandResultMsg{Info: fmt.Sprintf("rejected %s", id)}
	}
}

func (m Model) trigger(domainID string, trigger map[string]any) tea.Cmd {
	return func() tea.Msg {
		s, err := m.client.TriggerSession(domainID, trigger)
		if err != nil {
			return tabs.CommandResultMsg{Err: err}
		}
		return tabs.CommandResultMsg{Info: fmt.Sprintf("triggered %s", s.ID)}
	}
}

func parseTriggerJSON(s string) (map[string]any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return map[string]any{}, nil
	}
	out := map[string]any{}
	if !strings.HasPrefix(s, "{") {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) == 2 {
			out[parts[0]] = parts[1]
			return out, nil
		}
		return nil, fmt.Errorf("expected JSON object or key=value")
	}
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return out, nil
}

