package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
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
	width     int
	height    int
	theme     common.Theme
	keys      KeyMap
	statusBar StatusBar
	help      components.Help

	activeTab int
	helpOpen  bool
	tabs      []tea.Model
}

// NewModel creates the root TUI model.
func NewModel(serverURL string) Model {
	th := common.NewTheme()
	return Model{
		serverURL: serverURL,
		theme:     th,
		keys:      DefaultKeyMap(),
		statusBar: NewStatusBar(th),
		help:      components.NewHelp(th.Help),
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
	}
	for _, t := range m.tabs {
		cmds = append(cmds, t.Init())
	}
	return tea.Batch(cmds...)
}

// Update handles global input and delegates to the active tab.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.helpOpen {
			if keyMatches(msg, m.keys.Back, m.keys.Help, m.keys.Quit) {
				m.helpOpen = false
				return m, nil
			}
			return m, nil
		}

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
		}

		// Number keys 1-7 switch tabs directly.
		if n := tabNumber(msg.String()); n > 0 && n <= len(tabNames) {
			m.activeTab = n - 1
			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		for i := range m.tabs {
			m.tabs[i], _ = m.tabs[i].Update(msg)
		}
		return m, nil

	case tickMsg:
		return m, tickCmd()
	}

	// Delegate to active tab.
	var cmd tea.Cmd
	m.tabs[m.activeTab], cmd = m.tabs[m.activeTab].Update(msg)
	return m, cmd
}

// View renders the full TUI layout.
func (m Model) View() string {
	if m.width < 1 || m.height < 1 {
		return "Initializing..."
	}

	// Header
	header := m.statusBar.View(m.width, m.serverURL, true)
	headerHeight := lipgloss.Height(header)

	// Tab bar
	tabBar := m.renderTabBar()
	tabBarHeight := lipgloss.Height(tabBar)

	// Footer
	footer := m.theme.Footer.Render(m.renderFooter())
	footerHeight := lipgloss.Height(footer)

	// Body height accounts for header + tab bar + footer.
	bodyHeight := m.height - headerHeight - tabBarHeight - footerHeight
	if bodyHeight < 0 {
		bodyHeight = 0
	}

	// Resize active tab view to body area.
	bodyMsg := tea.WindowSizeMsg{Width: m.width, Height: bodyHeight}
	active := m.tabs[m.activeTab]
	active, _ = active.Update(bodyMsg)
	body := active.View()

	// Stack vertically.
	content := lipgloss.JoinVertical(lipgloss.Left, header, tabBar, body, footer)

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
	return fmt.Sprintf("tab:%d/%d  │  ↑↓/jk 选择  │  enter 详情  │  ? help  │  q quit", m.activeTab+1, len(tabNames))
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
		"/        过滤",
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

var tickInterval = 2 * time.Second

// SetTickInterval is used by tests to speed up or slow down ticks.
func SetTickInterval(d time.Duration) {
	tickInterval = d
}
