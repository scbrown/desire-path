package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

func seedInspectDesires(t *testing.T, s *store.SQLiteStore) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)

	desires := []model.Desire{
		{ID: "i1", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"/etc/hosts"}`), Error: "unknown tool", Source: "claude-code", Timestamp: base},
		{ID: "i2", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"/etc/passwd"}`), Error: "unknown tool", Source: "claude-code", Timestamp: base.Add(time.Hour)},
		{ID: "i3", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"/etc/hosts"}`), Error: "tool not found", Source: "cursor", Timestamp: base.Add(25 * time.Hour)},
		{ID: "i4", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"/tmp/test"}`), Error: "unknown tool", Source: "claude-code", Timestamp: base.Add(49 * time.Hour)},
		{ID: "i5", ToolName: "write_file", ToolInput: json.RawMessage(`{"path":"/tmp/out"}`), Error: "not allowed", Source: "claude-code", Timestamp: base.Add(2 * time.Hour)},
	}
	for _, d := range desires {
		if err := s.RecordDesire(ctx, d); err != nil {
			t.Fatalf("RecordDesire %s: %v", d.ID, err)
		}
	}
}

// resetFlags resets global flag state that persists between cobra executions.
func resetFlags(t *testing.T) {
	t.Helper()
	jsonOutput = false
	exportType = "desires"
}

func TestInspectCmdBasic(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	seedInspectDesires(t, s)
	s.Close()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"inspect", "read_file", "--db", db})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Pattern:    read_file") {
		t.Errorf("expected pattern header, got:\n%s", out)
	}
	if !strings.Contains(out, "Total:      4") {
		t.Errorf("expected total count of 4, got:\n%s", out)
	}
	if !strings.Contains(out, "Frequency:") {
		t.Error("expected frequency histogram section")
	}
	if !strings.Contains(out, "Top inputs:") {
		t.Error("expected top inputs section")
	}
	if !strings.Contains(out, "Top errors:") {
		t.Error("expected top errors section")
	}
	if !strings.Contains(out, "2026-01-10") {
		t.Error("expected first day in histogram")
	}
	if !strings.Contains(out, "/etc/hosts") {
		t.Error("expected /etc/hosts in top inputs")
	}
	if !strings.Contains(out, "unknown tool") {
		t.Error("expected 'unknown tool' in top errors")
	}
}

func TestInspectCmdJSON(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	seedInspectDesires(t, s)
	s.Close()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"inspect", "read_file", "--db", db, "--json"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var result store.InspectResult
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal JSON: %v\noutput: %s", err, buf.String())
	}
	if result.Total != 4 {
		t.Errorf("Total = %d, want 4", result.Total)
	}
	if result.Pattern != "read_file" {
		t.Errorf("Pattern = %q, want %q", result.Pattern, "read_file")
	}
	if len(result.Histogram) == 0 {
		t.Error("expected non-empty histogram")
	}
	if len(result.TopInputs) == 0 {
		t.Error("expected non-empty top inputs")
	}
	if len(result.TopErrors) == 0 {
		t.Error("expected non-empty top errors")
	}
}

func TestInspectCmdNoMatch(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	s.Close()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"inspect", "nonexistent_tool", "--db", db})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Total:      0") {
		t.Errorf("expected total 0 for nonexistent tool, got:\n%s", out)
	}
	if !strings.Contains(out, "No desires found") {
		t.Errorf("expected 'No desires found' message, got:\n%s", out)
	}
}

func TestInspectCmdWithAlias(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	seedInspectDesires(t, s)
	if err := s.SetAlias(context.Background(), "read_file", "Read"); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}
	s.Close()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"inspect", "read_file", "--db", db})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Alias:      Read") {
		t.Errorf("expected alias display, got:\n%s", out)
	}
}

func TestInspectCmdWildcard(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	seedInspectDesires(t, s)
	s.Close()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"inspect", "%_file", "--db", db})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	out := buf.String()
	// Should match both read_file (4) and write_file (1) = 5 total.
	if !strings.Contains(out, "Total:      5") {
		t.Errorf("expected total 5 for wildcard, got:\n%s", out)
	}
}

func TestInspectCmdRequiresArg(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"inspect", "--db", db})

	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing argument")
	}
}
