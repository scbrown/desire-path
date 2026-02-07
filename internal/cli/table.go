package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"
)

const defaultTermWidth = 80

// getTermWidth returns the current terminal width, defaulting to 80.
func getTermWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return defaultTermWidth
}

// isTTY reports whether w is connected to a terminal.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// bold wraps s in ANSI bold escape codes.
func bold(s string, color bool) string {
	if !color {
		return s
	}
	return "\033[1m" + s + "\033[0m"
}

// truncate shortens a string to max characters, appending "..." if truncated.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max < 4 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

// Table writes column-aligned output using text/tabwriter with consistent
// formatting across all commands. Headers are bold when output is a TTY.
type Table struct {
	tw    *tabwriter.Writer
	color bool
	width int
}

// NewTable creates a Table that writes to w. If headers are provided, they are
// written as a bold header row (bold only when w is a TTY).
func NewTable(w io.Writer, headers ...string) *Table {
	color := isTTY(w)
	width := defaultTermWidth
	if color {
		width = getTermWidth()
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	t := &Table{tw: tw, color: color, width: width}

	if len(headers) > 0 {
		row := make([]string, len(headers))
		for i, h := range headers {
			row[i] = bold(h, color)
		}
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	return t
}

// Row writes a data row with tab-separated values.
func (t *Table) Row(vals ...string) {
	fmt.Fprintln(t.tw, strings.Join(vals, "\t"))
}

// Flush flushes the underlying tabwriter.
func (t *Table) Flush() error {
	return t.tw.Flush()
}

// Bold wraps text in ANSI bold if color is enabled for this table.
func (t *Table) Bold(s string) string {
	return bold(s, t.color)
}

// Color reports whether color output is enabled.
func (t *Table) Color() bool {
	return t.color
}

// Width returns the detected terminal width.
// Returns defaultTermWidth (80) when output is not a TTY.
func (t *Table) Width() int {
	return t.width
}
