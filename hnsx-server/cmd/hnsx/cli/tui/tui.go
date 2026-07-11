// Package tui implements the terminal user interface for hnsx.
//
// TUI is not a subcommand; it is the default interactive surface when hnsx is
// run without arguments in a terminal. Explicit commands (e.g.
// `hnsx session list`) continue to work as scriptable alternatives.
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Run starts the TUI and blocks until the user quits. serverURL is the base
// URL of the hnsx-server to connect to.
func Run(serverURL string) error {
	m := NewModel(serverURL)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
