package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Help renders a centered help overlay with the given lines.
type Help struct {
	Style lipgloss.Style
}

// NewHelp creates a help component using the provided style.
func NewHelp(style lipgloss.Style) Help {
	return Help{Style: style}
}

// View renders the help box. Width and height are the terminal dimensions.
func (h Help) View(width, height int, lines []string) string {
	if width < 1 || height < 1 {
		return ""
	}
	content := strings.Join(lines, "\n")
	box := h.Style.Render(content)
	boxWidth := lipgloss.Width(box)
	boxHeight := lipgloss.Height(box)

	padTop := (height - boxHeight) / 2
	if padTop < 0 {
		padTop = 0
	}
	padLeft := (width - boxWidth) / 2
	if padLeft < 0 {
		padLeft = 0
	}

	return lipgloss.NewStyle().
		PaddingTop(padTop).
		PaddingLeft(padLeft).
		Render(box)
}
