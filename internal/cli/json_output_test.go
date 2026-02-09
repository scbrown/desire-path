package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

// captureStdoutAndStderr runs fn while capturing both stdout and stderr.
func captureStdoutAndStderr(t *testing.T, fn func()) (stdout, stderr string) {
	t.Helper()

	oldOut := os.Stdout
	oldErr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	fn()

	wOut.Close()
	wErr.Close()
	os.Stdout = oldOut
	os.Stderr = oldErr

	var bufOut, bufErr bytes.Buffer
	io.Copy(&bufOut, rOut)
	io.Copy(&bufErr, rErr)
	return bufOut.String(), bufErr.String()
}

func TestRecordCmdJSON(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")

	// Pipe JSON input to stdin.
	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		w.Write([]byte(`{"tool_name":"read_file","error":"unknown tool"}`))
		w.Close()
	}()

	stdout, _ := captureStdoutAndStderr(t, func() {
		rootCmd.SetArgs([]string{"record", "--source", "claude-code", "--db", db, "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	os.Stdin = oldStdin

	var inv model.Invocation
	if err := json.Unmarshal([]byte(stdout), &inv); err != nil {
		t.Fatalf("unmarshal JSON output: %v\noutput: %s", err, stdout)
	}
	if inv.ToolName != "read_file" {
		t.Errorf("ToolName = %q, want %q", inv.ToolName, "read_file")
	}
	if inv.Error != "unknown tool" {
		t.Errorf("Error = %q, want %q", inv.Error, "unknown tool")
	}
	if inv.ID == "" {
		t.Error("ID should be auto-generated")
	}
}

func TestRecordCmdNoJSON(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")

	oldStdin := os.Stdin
	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		w.Write([]byte(`{"tool_name":"read_file","error":"unknown tool"}`))
		w.Close()
	}()

	stdout, stderr := captureStdoutAndStderr(t, func() {
		rootCmd.SetArgs([]string{"record", "--source", "claude-code", "--db", db})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	os.Stdin = oldStdin

	if stdout != "" {
		t.Errorf("expected no stdout without --json, got: %s", stdout)
	}
	if !strings.Contains(stderr, "Recorded invocation:") {
		t.Errorf("expected 'Recorded invocation:' on stderr, got: %s", stderr)
	}
}

func TestRecordCmdRequiresSource(t *testing.T) {
	resetFlags(t)

	rootCmd.SetArgs([]string{"record"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when no --source specified")
	}
	if !strings.Contains(err.Error(), "--source flag is required") {
		t.Errorf("error should mention --source, got: %v", err)
	}
}

func TestAliasCmdSetJSON(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")

	stdout, _ := captureStdoutAndStderr(t, func() {
		rootCmd.SetArgs([]string{"alias", "--db", db, "--json", "read_file", "Read"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var result aliasResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("unmarshal JSON output: %v\noutput: %s", err, stdout)
	}
	if result.Action != "set" {
		t.Errorf("Action = %q, want %q", result.Action, "set")
	}
	if result.From != "read_file" {
		t.Errorf("From = %q, want %q", result.From, "read_file")
	}
	if result.To != "Read" {
		t.Errorf("To = %q, want %q", result.To, "Read")
	}
}

func TestAliasCmdDeleteJSON(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")

	// Seed an alias.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), "read_file", "Read"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	stdout, _ := captureStdoutAndStderr(t, func() {
		aliasDelete = true
		defer func() { aliasDelete = false }()
		rootCmd.SetArgs([]string{"alias", "--db", db, "--json", "--delete", "read_file"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var result aliasResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("unmarshal JSON output: %v\noutput: %s", err, stdout)
	}
	if result.Action != "deleted" {
		t.Errorf("Action = %q, want %q", result.Action, "deleted")
	}
	if result.From != "read_file" {
		t.Errorf("From = %q, want %q", result.From, "read_file")
	}
}

func TestAliasesCmdEmptyJSON(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	stdout, _ := captureStdoutAndStderr(t, func() {
		rootCmd.SetArgs([]string{"aliases", "--db", db, "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var aliases []model.Alias
	if err := json.Unmarshal([]byte(stdout), &aliases); err != nil {
		t.Fatalf("unmarshal JSON output: %v\noutput: %s", err, stdout)
	}
	if len(aliases) != 0 {
		t.Errorf("expected empty array, got %d aliases", len(aliases))
	}
}

func TestListCmdEmptyJSON(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	stdout, _ := captureStdoutAndStderr(t, func() {
		rootCmd.SetArgs([]string{"list", "--db", db, "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var desires []model.Desire
	if err := json.Unmarshal([]byte(stdout), &desires); err != nil {
		t.Fatalf("unmarshal JSON output: %v\noutput: %s", err, stdout)
	}
	if len(desires) != 0 {
		t.Errorf("expected empty array, got %d desires", len(desires))
	}
}

func TestListCmdEmptyTextToStderr(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	_, stderr := captureStdoutAndStderr(t, func() {
		rootCmd.SetArgs([]string{"list", "--db", db})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(stderr, "No desires found.") {
		t.Errorf("expected 'No desires found.' on stderr, got: %s", stderr)
	}
}

func TestExportCmdJSONFlag(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	seedExportDB(t, s)
	s.Close()

	stdout, _ := captureStdoutAndStderr(t, func() {
		rootCmd.SetArgs([]string{"export", "--db", db, "--json", "--format", "csv"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	// --json should override --format csv and produce JSON output.
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	for i, line := range lines {
		var d model.Desire
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			t.Errorf("line %d: expected valid JSON, got error: %v\nline: %s", i, err, line)
		}
	}
}

func TestInitCmdJSON(t *testing.T) {
	resetFlags(t)

	stdout, _ := captureStdoutAndStderr(t, func() {
		jsonOutput = true
		defer func() { jsonOutput = false }()
		if err := runInit("claude-code", false, ""); err != nil {
			t.Fatalf("runInit: %v", err)
		}
	})

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("unmarshal JSON output: %v\noutput: %s", err, stdout)
	}
	status, _ := result["status"].(string)
	if status != "configured" && status != "already_configured" {
		t.Errorf("status = %v, want %q or %q", result["status"], "configured", "already_configured")
	}
	if result["source"] != "claude-code" {
		t.Errorf("source = %v, want %q", result["source"], "claude-code")
	}
}

func TestPathsCmdEmptyJSON(t *testing.T) {
	resetFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	stdout, _ := captureStdoutAndStderr(t, func() {
		rootCmd.SetArgs([]string{"paths", "--db", db, "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	out := strings.TrimSpace(stdout)
	if out != "[]" {
		t.Errorf("expected [] for empty paths JSON, got %q", out)
	}
}
