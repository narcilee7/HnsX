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
		maxHeight: 7,
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

// View renders the command list.
func (c *CommandList) View() string {
	if !c.Visible() {
		return ""
	}

	visibleCount := len(c.filtered)
	if visibleCount > c.maxHeight {
		visibleCount = c.maxHeight
	}

	var lines []string
	for i := 0; i < visibleCount; i++ {
		cmd := c.filtered[i]
		name := "/" + cmd.Name
		if cmd.Args != "" {
			name += " " + cmd.Args
		}
		line := name
		if c.width > 0 {
			descStyle := c.theme.Muted
			desc := descStyle.Render(cmd.Description)
			// Leave some padding between name and description.
			nameWidth := lipgloss.Width(name)
			descWidth := lipgloss.Width(desc)
			available := c.width - nameWidth - descWidth - 4
			if available > 0 {
				gap := strings.Repeat(" ", available)
				line = name + gap + cmd.Description
			}
		}
		if i == c.selected {
			line = c.theme.RowSelected.Render(line)
		} else {
			line = c.theme.RowNormal.Render(line)
		}
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return ""
	}

	return lipgloss.NewStyle().
		Width(c.width).
		Background(lipgloss.Color("#2B2A28")).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

func (c *CommandList) filter() {
	c.selected = 0
	input := strings.TrimSpace(c.input)
	input = strings.TrimPrefix(input, "/")
	input = strings.ToLower(input)

	if input == "" {
		c.filtered = make([]Command, len(c.commands))
		copy(c.filtered, c.commands)
		return
	}

	c.filtered = c.filtered[:0]
	for _, cmd := range c.commands {
		if strings.HasPrefix(strings.ToLower(cmd.Name), input) {
			c.filtered = append(c.filtered, cmd)
		}
	}
}
