package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/config"
	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

func TestPaveCorrectNoMatch(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	// Rule for scp, but we send a different command.
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "r", To: "R", Tool: "Bash", Param: "command", Command: "scp", MatchKind: "flag",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	payload := `{"tool_name":"Bash","tool_input":{"command":"ls -la"},"error":"ls: cannot access '/nowhere': No such file or directory"}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCorrect(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCorrect: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if output != "" {
		t.Errorf("expected no output for non-matching command, got: %s", output)
	}
}

func TestPaveCorrectNonBashTool(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	// Read tool failures are not handled by pave-correct.
	payload := `{"tool_name":"Read","tool_input":{"file_path":"/tmp/nofile"},"error":"file not found"}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCorrect(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCorrect: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	if buf.String() != "" {
		t.Errorf("expected no output for non-Bash tool, got: %s", buf.String())
	}
}

func TestPaveCorrectInjectOnly(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "r", To: "R", Tool: "Bash", Param: "command", Command: "scp", MatchKind: "flag",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	// Ensure auto_correct is off.
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.toml")
	cfg := &config.Config{AutoCorrect: false}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	configPath = cfgPath
	defer func() { configPath = config.Path() }()

	payload := `{"tool_name":"Bash","tool_input":{"command":"scp -r file.txt host:/"},"error":"unknown flag: -r"}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCorrect(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCorrect: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result postFailureOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}

	ctx := result.HookSpecificOutput.AdditionalContext
	if !strings.Contains(ctx, "Auto-correction available") {
		t.Errorf("expected 'Auto-correction available' in context, got: %s", ctx)
	}
	if !strings.Contains(ctx, "Corrected command:") {
		t.Errorf("expected 'Corrected command:' in context, got: %s", ctx)
	}
	// Should NOT contain "Auto-executed" since auto_correct is off.
	if strings.Contains(ctx, "Auto-executed") {
		t.Errorf("should not auto-execute when auto_correct is off, got: %s", ctx)
	}
}

func TestPaveCorrectAutoExecute(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "grep", To: "echo", Tool: "Bash", Param: "command", Command: "grep", MatchKind: "command",
		Message: "test substitution",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	// Enable auto_correct.
	cfgDir := t.TempDir()
	cfgPath := filepath.Join(cfgDir, "config.toml")
	cfg := &config.Config{AutoCorrect: true}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}
	configPath = cfgPath
	defer func() { configPath = config.Path() }()

	payload := `{"tool_name":"Bash","tool_input":{"command":"grep hello world"},"error":"grep: world: No such file or directory"}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCorrect(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCorrect: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result postFailureOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}

	ctx := result.HookSpecificOutput.AdditionalContext
	if !strings.Contains(ctx, "Auto-correction available") {
		t.Errorf("expected correction description, got: %s", ctx)
	}
	if !strings.Contains(ctx, "Auto-executed corrected command") {
		t.Errorf("expected 'Auto-executed' in context, got: %s", ctx)
	}
	if !strings.Contains(ctx, "Result:") {
		t.Errorf("expected 'Result:' in context, got: %s", ctx)
	}
	// The corrected command is "echo hello world", so output should contain "hello world".
	if !strings.Contains(ctx, "hello world") {
		t.Errorf("expected 'hello world' in result, got: %s", ctx)
	}
}

func TestPaveCorrectInvalidJSON(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	stdin := strings.NewReader("not json")
	err = runPaveCorrect(stdin)
	if err != nil {
		t.Fatalf("expected nil for invalid JSON, got: %v", err)
	}
}

func TestPaveCorrectCommandRegistered(t *testing.T) {
	rootCmd.SetArgs([]string{"pave-correct", "--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("pave-correct help: %v", err)
	}
}
