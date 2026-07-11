package tabs

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/common"
)

// DashboardTab shows summary cards and a 24h cost sparkline.
type DashboardTab struct {
	client *common.Client

	width  int
	height int
	theme  common.Theme

	summary *common.DashboardSummary
	err     error
}

// NewDashboardTab creates a dashboard tab.
func NewDashboardTab(serverURL string) DashboardTab {
	th := common.NewTheme()
	return DashboardTab{
		client: common.NewClient(serverURL),
		theme:  th,
	}
}

// Init starts polling.
func (t DashboardTab) Init() tea.Cmd {
	return tea.Batch(t.fetchSummary(), tickDashboard())
}

// Update handles polling.
func (t DashboardTab) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		t.width = msg.Width
		t.height = msg.Height
		return t, nil
	case dashboardLoadedMsg:
		t.summary = msg.summary
		t.err = msg.err
		return t, nil
	case RefreshMsg:
		return t, t.fetchSummary()
	case tickMsg:
		return t, tea.Batch(t.fetchSummary(), tickDashboard())
	}
	return t, nil
}

// View renders dashboard cards and a sparkline.
func (t DashboardTab) View() string {
	if t.width < 1 || t.height < 1 {
		return "Loading dashboard..."
	}
	if t.summary == nil {
		return t.theme.Muted.Render("No data")
	}

	cardStyle := lipgloss.NewStyle().
		Width(t.width/2 - 2).
		Height(6).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(t.theme.TabActive.GetBackground()).
		Padding(1)

	cards := lipgloss.JoinHorizontal(lipgloss.Top,
		cardStyle.Render(card("Running Sessions", fmt.Sprintf("%d", t.summary.RunningSessions), t.theme.Badge["info"])),
		cardStyle.Render(card("24h Sessions", fmt.Sprintf("%d", t.summary.TotalSessions24h), t.theme.Badge["success"])),
	)
	cards2 := lipgloss.JoinHorizontal(lipgloss.Top,
		cardStyle.Render(card("24h Cost", fmt.Sprintf("$%.4f", t.summary.Cost24h), t.theme.Badge["warning"])),
		cardStyle.Render(card("24h Failure Rate", fmt.Sprintf("%.1f%%", t.summary.FailureRate*100), t.theme.Badge["danger"])),
	)

	spark := sparkline(t.summary.Cost24h, t.width-4)

	body := lipgloss.JoinVertical(lipgloss.Left, cards, cards2, "", t.theme.Title.Render("24h Cost Sparkline"), spark)
	if t.err != nil {
		body = lipgloss.JoinVertical(lipgloss.Left, t.theme.Badge["danger"].Render(fmt.Sprintf("error: %v", t.err)), body)
	}
	return body
}

func (t DashboardTab) fetchSummary() tea.Cmd {
	return func() tea.Msg {
		summary, err := t.client.DashboardSummary()
		return dashboardLoadedMsg{summary: summary, err: err}
	}
}

func tickDashboard() tea.Cmd {
	return tickSessions()
}

type dashboardLoadedMsg struct {
	summary *common.DashboardSummary
	err     error
}

func card(title, value string, valueStyle lipgloss.Style) string {
	return lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Foreground(lipgloss.Color("#9C9C92")).Render(title),
		"",
		valueStyle.Bold(true).Render(value),
	)
}

// sparkline renders a trivial sparkline from a single value. A real
// implementation would bucket hourly costs; this is a placeholder visual.
func sparkline(value float64, width int) string {
	if width < 1 {
		return ""
	}
	bar := "●"
	count := int(value * 100)
	if count > width {
		count = width
	}
	if count < 1 {
		count = 1
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#7E8FB0")).Render(strings.Repeat(bar, count))
}
