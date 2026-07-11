package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Output renders values according to the configured output mode.
type Output struct {
	mode string
	w    io.Writer
}

// NewOutput creates an Output bound to mode ("human", "json", "quiet").
func NewOutput(mode string) *Output {
	return &Output{mode: mode, w: os.Stdout}
}

// Mode returns the active output mode.
func (o *Output) Mode() string { return o.mode }

// Print writes a structured value.
//   - human: pretty JSON for ad-hoc debug; tabular rendering is per-command.
//   - json:  indented JSON (line-delimited in stream contexts).
//   - quiet: suppresses.
func (o *Output) Print(v any) {
	if o.mode == "quiet" {
		return
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal: %v\n", err)
		return
	}
	fmt.Fprintln(o.w, string(b))
}

// Line writes a single human-readable line (suppressed in quiet mode).
func (o *Output) Line(format string, args ...any) {
	if o.mode == "quiet" {
		return
	}
	fmt.Fprintf(o.w, format+"\n", args...)
}

// Error writes a structured error to stderr.
func (o *Output) Error(action string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
}

// Table prints a simple aligned table from headers + rows. Used for
// human-mode rendering in v0.3. v0.4+ may swap to a richer renderer.
func (o *Output) Table(headers []string, rows [][]string) {
	if o.mode == "quiet" {
		return
	}
	if o.mode == "json" {
		out := make([]map[string]string, 0, len(rows))
		for _, r := range rows {
			m := make(map[string]string, len(headers))
			for i, h := range headers {
				if i < len(r) {
					m[h] = r[i]
				}
			}
			out = append(out, m)
		}
		o.Print(out)
		return
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if i < len(widths) && len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	printRow := func(cells []string) {
		for i, c := range cells {
			if i >= len(widths) {
				continue
			}
			if i > 0 {
				fmt.Fprint(o.w, "  ")
			}
			fmt.Fprintf(o.w, "%-*s", widths[i], c)
		}
		fmt.Fprintln(o.w)
	}
	printRow(headers)
	for i := range headers {
		if i > 0 {
			fmt.Fprint(o.w, "  ")
		}
		fmt.Fprint(o.w, strings.Repeat("-", widths[i]))
	}
	fmt.Fprintln(o.w)
	for _, r := range rows {
		printRow(r)
	}
}