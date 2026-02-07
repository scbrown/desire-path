package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewTableHeaders(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "NAME", "VALUE", "STATUS")
	tbl.Row("foo", "42", "ok")
	tbl.Row("bar", "99", "error")
	tbl.Flush()

	out := buf.String()

	// Headers should be present (no bold for buffer since not a TTY).
	if !strings.Contains(out, "NAME") {
		t.Error("missing NAME header")
	}
	if !strings.Contains(out, "VALUE") {
		t.Error("missing VALUE header")
	}
	if !strings.Contains(out, "STATUS") {
		t.Error("missing STATUS header")
	}

	// Data should be present.
	if !strings.Contains(out, "foo") {
		t.Error("missing foo data")
	}
	if !strings.Contains(out, "99") {
		t.Error("missing 99 data")
	}

	// Should have 3 lines (header + 2 data rows).
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
}

func TestNewTableNoHeaders(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf)
	tbl.Row("a", "b")
	tbl.Flush()

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line (data only), got %d", len(lines))
	}
}

func TestNewTableAlignment(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "SHORT", "LONGER_HEADER")
	tbl.Row("a", "x")
	tbl.Row("longvalue", "y")
	tbl.Flush()

	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	// Column alignment: second column should start at the same position
	// for all lines (tabwriter aligns them).
	headerIdx := strings.Index(lines[0], "LONGER_HEADER")
	row2Idx := strings.Index(lines[2], "y")
	if headerIdx < 0 || row2Idx < 0 {
		t.Fatal("missing expected content")
	}
	if headerIdx != row2Idx {
		t.Errorf("columns not aligned: header col2 at %d, row2 col2 at %d", headerIdx, row2Idx)
	}
}

func TestIsTTYBuffer(t *testing.T) {
	var buf bytes.Buffer
	if isTTY(&buf) {
		t.Error("bytes.Buffer should not be a TTY")
	}
}

func TestTableColorDisabledForBuffer(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf, "HEADER")
	if tbl.Color() {
		t.Error("color should be disabled for buffer output")
	}

	// Bold should be a no-op.
	s := tbl.Bold("test")
	if s != "test" {
		t.Errorf("Bold should be no-op for non-TTY, got %q", s)
	}
}

func TestTableWidthDefault(t *testing.T) {
	var buf bytes.Buffer
	tbl := NewTable(&buf)
	if tbl.Width() != defaultTermWidth {
		t.Errorf("width = %d, want %d", tbl.Width(), defaultTermWidth)
	}
}

func TestBoldWithColor(t *testing.T) {
	s := bold("hello", true)
	if !strings.Contains(s, "\033[1m") {
		t.Error("expected ANSI bold start")
	}
	if !strings.Contains(s, "\033[0m") {
		t.Error("expected ANSI reset")
	}
	if !strings.Contains(s, "hello") {
		t.Error("expected content preserved")
	}
}

func TestBoldWithoutColor(t *testing.T) {
	s := bold("hello", false)
	if s != "hello" {
		t.Errorf("bold without color should be identity, got %q", s)
	}
}

func TestGetTermWidthFallback(t *testing.T) {
	// In test environment, stdout is typically not a terminal.
	// getTermWidth should return the default.
	w := getTermWidth()
	if w <= 0 {
		t.Errorf("getTermWidth() = %d, want > 0", w)
	}
}
