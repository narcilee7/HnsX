package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// morandi palette, kept in sync with observability/src/tokens/morandi.css.
var (
	primary   = lipgloss.Color("#7A8B7F")
	success   = lipgloss.Color("#7E9B7A")
	warning   = lipgloss.Color("#C9A87C")
	danger    = lipgloss.Color("#B57F7F")
	info      = lipgloss.Color("#7E8FB0")
	muted     = lipgloss.Color("#9C9C92")
	bgLight   = lipgloss.Color("#F5F1EA")
	bgDark    = lipgloss.Color("#2B2A28")
	fgLight   = lipgloss.Color("#2B2A28")
	fgDark    = lipgloss.Color("#F5F1EA")
)

// Theme holds the terminal styles used across the TUI.
type Theme struct {
	Title       lipgloss.Style
	TabActive   lipgloss.Style
	TabInactive lipgloss.Style
	RowSelected lipgloss.Style
	RowNormal   lipgloss.Style
	Footer      lipgloss.Style
	Header      lipgloss.Style
	Muted       lipgloss.Style
	Help        lipgloss.Style
	Badge       map[string]lipgloss.Style
}

// NewTheme returns a theme that adapts to the terminal background. For now we
// always use the dark palette; light mode can be enabled later via NO_COLOR or
// an explicit flag.
func NewTheme() Theme {
	bg := bgDark
	fg := fgDark

	t := Theme{
		Title: lipgloss.NewStyle().
			Bold(true).
			Foreground(primary).
			PaddingLeft(1).
			PaddingRight(1),
		TabActive: lipgloss.NewStyle().
			Bold(true).
			Foreground(bg).
			Background(primary).
			Padding(0, 1),
		TabInactive: lipgloss.NewStyle().
			Foreground(muted).
			Padding(0, 1),
		RowSelected: lipgloss.NewStyle().
			Foreground(fg).
			Background(info).
			Padding(0, 1),
		RowNormal: lipgloss.NewStyle().
			Foreground(fg).
			Padding(0, 1),
		Footer: lipgloss.NewStyle().
			Foreground(muted).
			Background(lipgloss.Color("#3A3937")).
			Padding(0, 1),
		Header: lipgloss.NewStyle().
			Foreground(fg).
			Background(lipgloss.Color("#3A3937")).
			Padding(0, 1),
		Muted: lipgloss.NewStyle().
			Foreground(muted),
		Help: lipgloss.NewStyle().
			Foreground(fg).
			Background(bg).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(primary).
			Padding(1, 2),
		Badge: map[string]lipgloss.Style{
			"success": lipgloss.NewStyle().Foreground(success),
			"warning": lipgloss.NewStyle().Foreground(warning),
			"danger":  lipgloss.NewStyle().Foreground(danger),
			"info":    lipgloss.NewStyle().Foreground(info),
		},
	}
	return t
}

// DefaultWidth is the minimum recommended terminal width.
const DefaultWidth = 80
