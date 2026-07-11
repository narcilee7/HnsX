package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/hnsx-io/hnsx/server/cmd/hnsx/cli/tui/common"
)

// Command describes a single slash command available in the TUI command palette.
type Command struct {
	Name        string
	Args        string
	Description string
	NoArgs      bool
}

// DefaultCommands is the built-in command vocabulary for the TUI command palette.
var DefaultCommands = []Command{
	{Name: "help", Description: "show available commands", NoArgs: true},
	{Name: "quit", Description: "exit the TUI", NoArgs: true},
	{Name: "session", Args: "<id>", Description: "jump to session and tail it"},
	{Name: "trace", Args: "<id>", Description: "open trace detail"},
	{Name: "domain", Args: "<id>", Description: "jump to domain"},
	{Name: "approve", Args: "<id>", Description: "approve an approval"},
	{Name: "reject", Args: "<id> [reason]", Description: "reject an approval"},
	{Name: "trigger", Args: "<domain> [json]", Description: "trigger a session"},
	{Name: "filter", Args: "<text>", Description: "filter the current tab"},
	{Name: "refresh", Description: "refresh the current tab", NoArgs: true},
}

// CommandList is a filterable, selectable list of slash commands.
type CommandList struct {
	theme     common.Theme
	commands  []Command
	filtered  []Command
	selected  int
	input     string
	width     int
	maxHeight int
}

// NewCommandList creates a command list using the given theme.
func NewCommandList(theme common.Theme) CommandList {
	c := CommandList{
		theme:     theme,
		commands:  DefaultCommands,
		filtered:  make([]Command, len(DefaultCommands)),
		maxHeight: 5,
	}
	copy(c.filtered, c.commands)
	return c
}

// SetWidth sets the render width for the list.
func (c *CommandList) SetWidth(w int) {
	c.width = w
}

// SetMaxHeight sets the maximum number of visible rows.
func (c *CommandList) SetMaxHeight(h int) {
	c.maxHeight = h
}

// SetInput filters the list by the current command input (including the leading slash).
func (c *CommandList) SetInput(input string) {
	c.input = input
	c.filter()
}

// MoveUp moves the selection up.
func (c *CommandList) MoveUp() {
	if c.selected > 0 {
		c.selected--
	}
}

// MoveDown moves the selection down.
func (c *CommandList) MoveDown() {
	if c.selected < len(c.filtered)-1 {
		c.selected++
	}
}

// Selected returns the currently selected command, if any.
func (c *CommandList) Selected() (Command, bool) {
	if c.selected < 0 || c.selected >= len(c.filtered) {
		return Command{}, false
	}
	return c.filtered[c.selected], true
}

// HasSelection returns true when a non-empty command is selected.
func (c *CommandList) HasSelection() bool {
	_, ok := c.Selected()
	return ok
}

// Visible returns true when there are commands to show. Once the user has typed
// a space after a complete command name the list is hidden to make room for arg input.
func (c *CommandList) Visible() bool {
	// If input contains a space, the command name is complete; hide the list.
	if strings.Contains(c.input, " ") {
		return false
	}
	return len(c.filtered) > 0
}

// View renders the command list as a dropdown panel anchored above the command input.
func (c *CommandList) View() string {
	if !c.Visible() {
		return ""
	}

	visibleCount := len(c.filtered)
	if visibleCount > c.maxHeight {
		visibleCount = c.maxHeight
	}

	innerWidth := c.width - 2
	if innerWidth < 1 {
		innerWidth = c.width
	}

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#F5F1EA")).
		Background(lipgloss.Color("#3A3937")).
		Padding(0, 1).
		Render("Command")
	divider := strings.Repeat("─", innerWidth-lipgloss.Width(title))
	header := lipgloss.JoinHorizontal(lipgloss.Bottom, title, divider)

	var lines []string
	for i := 0; i < visibleCount; i++ {
		cmd := c.filtered[i]
		name := "/" + cmd.Name
		if cmd.Args != "" {
			name += " " + cmd.Args
		}

		nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F5F1EA"))
		descStyle := c.theme.Muted
		namePart := nameStyle.Render(name)
		descPart := descStyle.Render(cmd.Description)

		nameW := lipgloss.Width(namePart)
		descW := lipgloss.Width(descPart)
		gapW := innerWidth - nameW - descW - 2
		if gapW < 1 {
			gapW = 1
		}
		gap := strings.Repeat(" ", gapW)

		line := " " + name + gap + cmd.Description
		if i == c.selected {
			line = c.theme.RowSelected.Width(innerWidth).Render(line)
		} else {
			line = c.theme.RowNormal.Width(innerWidth).Render(line)
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return ""
	}

	content := lipgloss.JoinVertical(lipgloss.Left, append([]string{header}, lines...)...)
	return lipgloss.NewStyle().
		Width(c.width).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("#7A8B7F")).
		BorderTop(true).
		BorderLeft(true).
		BorderRight(true).
		Background(lipgloss.Color("#2B2A28")).
		Render(content)
}

func (c *CommandList) filter() {
	input := strings.TrimSpace(c.input)
	input = strings.TrimPrefix(input, "/")
	input = strings.ToLower(input)

	var prevName string
	if c.selected >= 0 && c.selected < len(c.filtered) {
		prevName = c.filtered[c.selected].Name
	}

	c.filtered = c.filtered[:0]
	for _, cmd := range c.commands {
		if strings.HasPrefix(strings.ToLower(cmd.Name), input) {
			c.filtered = append(c.filtered, cmd)
		}
	}

	// Preserve the previous selection when it still matches; otherwise reset to top.
	c.selected = 0
	for i, cmd := range c.filtered {
		if cmd.Name == prevName {
			c.selected = i
			break
		}
	}
}
