package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Table is a minimal generic table renderer for the TUI. It supports a header
// row, selectable rows, and horizontal column layout computed from the
// available width.
type Table struct {
	Style       lipgloss.Style
	HeaderStyle lipgloss.Style
	RowStyle    lipgloss.Style
	SelStyle    lipgloss.Style
	ColPadding  int
}

// NewTable creates a table with default styles.
func NewTable(theme TableTheme) Table {
	return Table{
		Style:       theme.Style,
		HeaderStyle: theme.Header,
		RowStyle:    theme.Row,
		SelStyle:    theme.Selected,
		ColPadding:  2,
	}
}

// TableTheme carries the styles used by a table.
type TableTheme struct {
	Style    lipgloss.Style
	Header   lipgloss.Style
	Row      lipgloss.Style
	Selected lipgloss.Style
}

// Row represents one table row.
type Row struct {
	Cells []string
}

// View renders the table.
//
// width: terminal width available for the table body.
// headers: column headers.
// rows: row data.
// selected: zero-based index of the selected row, or -1 if none.
func (t Table) View(width int, headers []string, rows []Row, selected int) string {
	if width < 1 || len(headers) == 0 {
		return ""
	}
	colWidths := t.calcColWidths(width, headers, rows)

	var lines []string
	lines = append(lines, t.renderRow(headers, colWidths, t.HeaderStyle, true))
	for i, r := range rows {
		style := t.RowStyle
		if i == selected {
			style = t.SelStyle
		}
		lines = append(lines, t.renderRow(r.Cells, colWidths, style, false))
	}
	return t.Style.Render(strings.Join(lines, "\n"))
}

func (t Table) renderRow(cells []string, widths []int, style lipgloss.Style, header bool) string {
	var parts []string
	for i, w := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		parts = append(parts, t.truncate(cell, w, style))
	}
	return style.Render(strings.Join(parts, strings.Repeat(" ", t.ColPadding)))
}

func (t Table) truncate(s string, width int, style lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	// Subtract padding that lipgloss may add; keep it simple for now.
	contentWidth := width
	if lipgloss.Width(s) <= contentWidth {
		return style.Width(contentWidth).Render(s)
	}
	if contentWidth <= 3 {
		return style.Width(contentWidth).Render(s[:contentWidth])
	}
	return style.Width(contentWidth).Render(s[:contentWidth-3] + "...")
}

func (t Table) calcColWidths(width int, headers []string, rows []Row) []int {
	// Distribute width evenly across columns, leaving room for padding.
	usable := width - (len(headers)-1)*t.ColPadding
	if usable < len(headers) {
		usable = len(headers)
	}
	base := usable / len(headers)
	extra := usable % len(headers)
	widths := make([]int, len(headers))
	for i := range headers {
		widths[i] = base
		if i < extra {
			widths[i]++
		}
	}
	return widths
}
