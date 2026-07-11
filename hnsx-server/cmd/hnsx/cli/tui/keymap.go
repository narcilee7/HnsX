package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap holds the global key bindings for the TUI.
type KeyMap struct {
	Quit    key.Binding
	Help    key.Binding
	Refresh key.Binding
	Filter  key.Binding
	Back    key.Binding
	NextTab key.Binding
	PrevTab key.Binding
	First   key.Binding
	Last    key.Binding
}

// DefaultKeyMap returns the default global key map.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		Back: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "back"),
		),
		NextTab: key.NewBinding(
			key.WithKeys("tab", "right"),
			key.WithHelp("tab", "next tab"),
		),
		PrevTab: key.NewBinding(
			key.WithKeys("shift+tab", "left"),
			key.WithHelp("shift+tab", "prev tab"),
		),
		First: key.NewBinding(
			key.WithKeys("g", "home"),
			key.WithHelp("g", "first"),
		),
		Last: key.NewBinding(
			key.WithKeys("G", "end"),
			key.WithHelp("G", "last"),
		),
	}
}
