package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
)

func TestWritePathsTable(t *testing.T) {
	paths := []model.Path{
		{
			ID:        "read_file",
			Pattern:   "read_file",
			Count:     42,
			FirstSeen: time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
			LastSeen:  time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC),
			AliasTo:   "Read",
		},
		{
			ID:        "write_file",
			Pattern:   "write_file",
			Count:     15,
			FirstSeen: time.Date(2026, 1, 20, 14, 0, 0, 0, time.UTC),
			LastSeen:  time.Date(2026, 2, 5, 9, 0, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	writePathsTable(&buf, paths)
	out := buf.String()

	// Verify header is present.
	if !strings.Contains(out, "RANK") {
		t.Error("missing RANK header")
	}
	if !strings.Contains(out, "PATTERN") {
		t.Error("missing PATTERN header")
	}
	if !strings.Contains(out, "ALIAS") {
		t.Error("missing ALIAS header")
	}

	// Verify data rows.
	if !strings.Contains(out, "read_file") {
		t.Error("missing read_file pattern")
	}
	if !strings.Contains(out, "write_file") {
		t.Error("missing write_file pattern")
	}
	if !strings.Contains(out, "42") {
		t.Error("missing count 42")
	}
	if !strings.Contains(out, "Read") {
		t.Error("missing alias Read")
	}

	// Verify rank ordering.
	readIdx := strings.Index(out, "read_file")
	writeIdx := strings.Index(out, "write_file")
	if readIdx > writeIdx {
		t.Error("read_file should appear before write_file")
	}
}

func TestWritePathsTableEmpty(t *testing.T) {
	var buf bytes.Buffer
	writePathsTable(&buf, nil)
	out := buf.String()

	// Should still have header.
	if !strings.Contains(out, "RANK") {
		t.Error("missing header for empty table")
	}
	// Should only have the header line.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 line (header only), got %d", len(lines))
	}
}

func TestWritePathsJSON(t *testing.T) {
	paths := []model.Path{
		{
			ID:        "read_file",
			Pattern:   "read_file",
			Count:     5,
			FirstSeen: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			LastSeen:  time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			AliasTo:   "Read",
		},
	}

	var buf bytes.Buffer
	if err := writePathsJSON(&buf, paths); err != nil {
		t.Fatalf("writePathsJSON: %v", err)
	}
	out := buf.String()

	if !strings.Contains(out, `"pattern": "read_file"`) {
		t.Error("missing pattern in JSON output")
	}
	if !strings.Contains(out, `"count": 5`) {
		t.Error("missing count in JSON output")
	}
	if !strings.Contains(out, `"alias_to": "Read"`) {
		t.Error("missing alias_to in JSON output")
	}
}

func TestWritePathsJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := writePathsJSON(&buf, nil); err != nil {
		t.Fatalf("writePathsJSON: %v", err)
	}
	out := strings.TrimSpace(buf.String())
	if out != "[]" {
		t.Errorf("expected [] for nil paths, got %q", out)
	}
}

func TestWritePathsTableTimestampFormat(t *testing.T) {
	paths := []model.Path{
		{
			ID:        "tool",
			Pattern:   "tool",
			Count:     1,
			FirstSeen: time.Date(2026, 3, 15, 8, 30, 0, 0, time.UTC),
			LastSeen:  time.Date(2026, 3, 15, 8, 30, 0, 0, time.UTC),
		},
	}

	var buf bytes.Buffer
	writePathsTable(&buf, paths)
	out := buf.String()

	// Timestamps should be RFC3339 format.
	if !strings.Contains(out, "2026-03-15T08:30:00Z") {
		t.Errorf("expected RFC3339 timestamp, got: %s", out)
	}
}
