package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/common"
)

// StatusBar renders the top bar: title, server URL, health, version, clock.
type StatusBar struct {
	Theme   common.Theme
	Version string
}

// NewStatusBar creates a status bar. Version may be empty until injected at
// build time.
func NewStatusBar(t common.Theme) StatusBar {
	return StatusBar{Theme: t}
}

// View renders the status bar for the given width and state.
func (s StatusBar) View(width int, serverURL string, serverOK bool) string {
	if width < 1 {
		return ""
	}
	status := s.Theme.Badge["danger"].Render("✗ offline")
	if serverOK {
		status = s.Theme.Badge["success"].Render("✓ ok")
	}

	left := s.Theme.Title.Render("HnsX")
	mid := s.Theme.Muted.Render(fmt.Sprintf("%s · server %s", serverURL, status))
	right := s.Theme.Muted.Render(time.Now().Format("15:04"))
	if s.Version != "" {
		right = s.Theme.Muted.Render(fmt.Sprintf("hnsx %s · %s", s.Version, time.Now().Format("15:04")))
	}

	// Fill the gap between left and mid, and mid and right.
	gapStyle := lipgloss.NewStyle().Background(lipgloss.Color("#3A3937"))
	available := width - lipgloss.Width(left) - lipgloss.Width(right)
	if available < lipgloss.Width(mid) {
		mid = s.Theme.Muted.Render(serverURL)
		available = width - lipgloss.Width(left) - lipgloss.Width(right)
	}
	if available < 0 {
		available = 0
	}
	gap := gapStyle.Width(available).Render("")
	return lipgloss.JoinHorizontal(lipgloss.Top, left, gap, right)
}
