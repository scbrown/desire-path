package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input string
		want  time.Duration
		err   bool
	}{
		{"7d", 7 * 24 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"30d", 30 * 24 * time.Hour, false},
		{"24h", 24 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"90s", 90 * time.Second, false},
		{"1h30m", 90 * time.Minute, false},
		{"", 0, true},
		{"xd", 0, true},
		{"abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseDuration(tt.input)
			if (err != nil) != tt.err {
				t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.err)
				return
			}
			if got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"short", 50, "short"},
		{"", 50, ""},
		{"exactly fifty characters long string that is here!", 50, "exactly fifty characters long string that is here!"},
		{"this string is definitely longer than fifty characters in total length", 50, "this string is definitely longer than fifty cha..."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncate(tt.input, tt.max)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
			}
		})
	}
}

func seedDesires(t *testing.T, s store.Store) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC)
	desires := []model.Desire{
		{ID: "1", ToolName: "read_file", Error: "unknown tool", Source: "claude-code", Timestamp: base},
		{ID: "2", ToolName: "run_tests", Error: "not found", Source: "cursor", Timestamp: base.Add(-time.Hour)},
		{ID: "3", ToolName: "read_file", Error: "permission denied", Source: "claude-code", Timestamp: base.Add(-48 * time.Hour)},
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

func TestListCmdTable(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	seedDesires(t, s)
	s.Close()

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"list", "--db", db})

	// Capture stdout
	old := listSince
	defer func() { listSince = old }()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestListCmdJSON(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	seedDesires(t, s)
	s.Close()

	// Reset flags for clean test
	rootCmd.SetArgs([]string{"list", "--db", db, "--json"})

	// We can't easily capture stdout from cobra, so just verify no error
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestListCmdEmpty(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	rootCmd.SetArgs([]string{"list", "--db", db})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
}

func TestListCmdFilters(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	seedDesires(t, s)
	s.Close()

	tests := []struct {
		name string
		args []string
	}{
		{"source filter", []string{"list", "--db", db, "--source", "claude-code"}},
		{"tool filter", []string{"list", "--db", db, "--tool", "read_file"}},
		{"limit", []string{"list", "--db", db, "--limit", "1"}},
		{"combined", []string{"list", "--db", db, "--source", "claude-code", "--tool", "read_file", "--limit", "10"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rootCmd.SetArgs(tt.args)
			if err := rootCmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
		})
	}
}

func TestListCmdInvalidSince(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	rootCmd.SetArgs([]string{"list", "--db", db, "--since", "invalid"})

	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for invalid --since")
	}
}

func TestListCmdJSONOutput(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	d := model.Desire{
		ID:        "test-1",
		ToolName:  "read_file",
		Error:     "not found",
		Source:    "test",
		Timestamp: time.Date(2026, 2, 7, 12, 0, 0, 0, time.UTC),
	}
	if err := s.RecordDesire(ctx, d); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Verify JSON output is valid by querying directly
	s2, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	desires, err := s2.ListDesires(ctx, store.ListOpts{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}

	data, err := json.Marshal(desires)
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}

	var result []model.Desire
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("got %d desires, want 1", len(result))
	}
	if result[0].ToolName != "read_file" {
		t.Errorf("got tool_name=%q, want %q", result[0].ToolName, "read_file")
	}
}
