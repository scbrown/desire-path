package cli

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

func seedExportDB(t *testing.T, s store.Store) {
	t.Helper()
	desires := []model.Desire{
		{
			ID:        "d1",
			ToolName:  "read_file",
			Error:     "unknown tool",
			Source:    "claude-code",
			Timestamp: time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			ID:        "d2",
			ToolName:  "run_tests",
			Error:     "not found",
			Source:    "claude-code",
			Timestamp: time.Date(2024, 7, 15, 8, 30, 0, 0, time.UTC),
		},
		{
			ID:        "d3",
			ToolName:  "deploy",
			Error:     "permission denied",
			Source:    "cursor",
			SessionID: "sess-abc",
			CWD:       "/home/user/project",
			Timestamp: time.Date(2024, 8, 20, 16, 0, 0, 0, time.UTC),
			ToolInput: json.RawMessage(`{"target":"prod"}`),
			Metadata:  json.RawMessage(`{"env":"staging"}`),
		},
	}
	for _, d := range desires {
		if err := s.RecordDesire(context.Background(), d); err != nil {
			t.Fatalf("seed desire %s: %v", d.ID, err)
		}
	}
}

func newTestStore(t *testing.T) store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.New(dir + "/test.db")
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// captureStdout runs fn while capturing stdout, returning the output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestParseSince(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"2024-01-01T00:00:00Z", false},
		{"2024-07-01", false},
		{"not-a-date", true},
		{"", true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			_, err := parseSince(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSince(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestWriteJSON(t *testing.T) {
	s := newTestStore(t)
	seedExportDB(t, s)

	desires, err := s.ListDesires(context.Background(), store.ListOpts{})
	if err != nil {
		t.Fatalf("list desires: %v", err)
	}

	output := captureStdout(t, func() {
		if err := writeJSON(desires); err != nil {
			t.Fatalf("writeJSON: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	// Each line should be valid JSON.
	for i, line := range lines {
		var d model.Desire
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Errorf("line %d: invalid JSON: %v", i, err)
		}
	}
}

func TestWriteCSV(t *testing.T) {
	s := newTestStore(t)
	seedExportDB(t, s)

	desires, err := s.ListDesires(context.Background(), store.ListOpts{})
	if err != nil {
		t.Fatalf("list desires: %v", err)
	}

	output := captureStdout(t, func() {
		if err := writeCSV(desires); err != nil {
			t.Fatalf("writeCSV: %v", err)
		}
	})

	r := csv.NewReader(strings.NewReader(output))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}

	// Header + 3 data rows.
	if len(records) != 4 {
		t.Fatalf("expected 4 CSV rows (1 header + 3 data), got %d", len(records))
	}

	header := records[0]
	expectedHeader := []string{"id", "tool_name", "error", "source", "session_id", "cwd", "timestamp", "tool_input", "metadata"}
	for i, h := range expectedHeader {
		if header[i] != h {
			t.Errorf("header[%d] = %q, want %q", i, header[i], h)
		}
	}
}

func TestExportCmdJSON(t *testing.T) {
	s := newTestStore(t)
	seedExportDB(t, s)
	_ = s.Close()

	// Re-use the db path through the CLI command.
	dbFile := t.TempDir() + "/cmd-test.db"
	s2, err := store.New(dbFile)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	seedExportDB(t, s2)
	s2.Close()

	// Test through cobra command execution.
	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"export", "--db", dbFile, "--format", "json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("export command: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 JSON lines, got %d: %s", len(lines), output)
	}
}

func TestExportCmdCSV(t *testing.T) {
	dbFile := t.TempDir() + "/cmd-csv-test.db"
	s, err := store.New(dbFile)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	seedExportDB(t, s)
	s.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"export", "--db", dbFile, "--format", "csv"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("export command: %v", err)
		}
	})

	r := csv.NewReader(strings.NewReader(output))
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("expected 4 CSV rows, got %d", len(records))
	}
}

func TestExportCmdSince(t *testing.T) {
	dbFile := t.TempDir() + "/cmd-since-test.db"
	s, err := store.New(dbFile)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	seedExportDB(t, s)
	s.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"export", "--db", dbFile, "--format", "json", "--since", "2024-08-01"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("export command: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line (only d3 after 2024-08-01), got %d: %s", len(lines), output)
	}

	var d model.Desire
	if err := json.Unmarshal([]byte(lines[0]), &d); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if d.ToolName != "deploy" {
		t.Errorf("expected tool_name=deploy, got %q", d.ToolName)
	}
}

func TestExportInvalidFormat(t *testing.T) {
	dbFile := t.TempDir() + "/cmd-badfmt-test.db"
	s, err := store.New(dbFile)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	s.Close()

	rootCmd.SetArgs([]string{"export", "--db", dbFile, "--format", "xml", "--since", ""})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for unsupported format, got nil")
	}
}

func TestExportInvalidSince(t *testing.T) {
	dbFile := t.TempDir() + "/cmd-badsince-test.db"
	s, err := store.New(dbFile)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	s.Close()

	rootCmd.SetArgs([]string{"export", "--db", dbFile, "--format", "json", "--since", "not-a-date"})
	err = rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --since, got nil")
	}
}

func TestExportEmpty(t *testing.T) {
	dbFile := t.TempDir() + "/cmd-empty-test.db"
	s, err := store.New(dbFile)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	s.Close()

	output := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"export", "--db", dbFile, "--format", "json", "--since", ""})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("export command: %v", err)
		}
	})

	if strings.TrimSpace(output) != "" {
		t.Errorf("expected empty output for empty db, got %q", output)
	}
}
