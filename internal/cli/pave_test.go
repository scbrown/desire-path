package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/store"
)

func TestPaveCheckMatchingAlias(t *testing.T) {
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

	dbPath = db

	// Simulate hook stdin with a matching tool name.
	payload := `{"tool_name":"read_file","tool_input":{"file_path":"/tmp/test"}}`
	stdin := strings.NewReader(payload)

	// runPaveCheck calls os.Exit(2) on match, so we test the logic
	// up to the point before exit by checking the store lookup directly.
	s2, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	alias, err := s2.GetAlias(context.Background(), "read_file")
	if err != nil {
		t.Fatalf("get alias: %v", err)
	}
	if alias == nil {
		t.Fatal("expected alias, got nil")
	}
	if alias.To != "Read" {
		t.Errorf("expected alias target 'Read', got %q", alias.To)
	}

	// Also verify the payload parses correctly.
	var p hookPayload
	if err := json.NewDecoder(stdin).Decode(&p); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if p.ToolName != "read_file" {
		t.Errorf("expected tool_name 'read_file', got %q", p.ToolName)
	}
}

func TestPaveCheckNoAlias(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	// Create empty store.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	// Simulate hook stdin with a tool name that has no alias.
	payload := `{"tool_name":"Read"}`
	stdin := strings.NewReader(payload)

	// runPaveCheck should return nil (allow the call).
	err = runPaveCheck(stdin)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestPaveCheckInvalidJSON(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	// Invalid JSON should not block the call.
	stdin := strings.NewReader("not json at all")
	err = runPaveCheck(stdin)
	if err != nil {
		t.Fatalf("expected nil error for invalid JSON, got: %v", err)
	}
}

func TestPaveCheckEmptyToolName(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	stdin := strings.NewReader(`{"tool_name":""}`)
	err = runPaveCheck(stdin)
	if err != nil {
		t.Fatalf("expected nil error for empty tool_name, got: %v", err)
	}
}

func TestPaveAgentsMD(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	// Seed aliases.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), "read_file", "Read"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), "search_files", "Grep"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	jsonOutput = false
	paveHook = false
	paveAgentsMD = true
	paveAppend = ""
	defer func() { paveAgentsMD = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"pave", "--db", db, "--agents-md"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "# Tool Name Corrections") {
		t.Errorf("expected header, got: %s", output)
	}
	if !strings.Contains(output, "Do NOT call `read_file`. Use `Read` instead.") {
		t.Errorf("expected read_file rule, got: %s", output)
	}
	if !strings.Contains(output, "Do NOT call `search_files`. Use `Grep` instead.") {
		t.Errorf("expected search_files rule, got: %s", output)
	}
}

func TestPaveAgentsMDAppend(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	// Seed an alias.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), "run_tests", "Bash"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	outFile := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(outFile, []byte("# Existing Content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath = db
	jsonOutput = false
	paveHook = false
	paveAgentsMD = true
	paveAppend = outFile
	defer func() { paveAgentsMD = false; paveAppend = "" }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"pave", "--db", db, "--agents-md", "--append", outFile})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	stdout := buf.String()

	if !strings.Contains(stdout, "Appended 1 rules") {
		t.Errorf("expected append confirmation, got: %s", stdout)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "# Existing Content") {
		t.Error("existing content was lost")
	}
	if !strings.Contains(content, "Do NOT call `run_tests`. Use `Bash` instead.") {
		t.Errorf("expected appended rule, got: %s", content)
	}
}

func TestPaveAgentsMDJSON(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), "read_file", "Read"); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	jsonOutput = true
	paveHook = false
	paveAgentsMD = true
	paveAppend = ""
	defer func() { jsonOutput = false; paveAgentsMD = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"pave", "--db", db, "--json", "--agents-md"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("json unmarshal: %v\nOutput: %s", err, buf.String())
	}
	if result["status"] != "generated" {
		t.Errorf("expected status 'generated', got %v", result["status"])
	}
	count, ok := result["count"].(float64)
	if !ok || count != 1 {
		t.Errorf("expected count 1, got %v", result["count"])
	}
}

func TestPaveAgentsMDNoAliases(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	jsonOutput = false
	paveHook = false
	paveAgentsMD = true
	paveAppend = ""
	defer func() { paveAgentsMD = false }()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	rootCmd.SetArgs([]string{"pave", "--db", db, "--agents-md"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stderr = oldStderr
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "No aliases configured.") {
		t.Errorf("expected 'No aliases configured.', got: %s", output)
	}
}

func TestPaveNoFlags(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db
	paveHook = false
	paveAgentsMD = false
	paveAppend = ""

	rootCmd.SetArgs([]string{"pave", "--db", db})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when no flags specified")
	}
	if !strings.Contains(err.Error(), "--hook or --agents-md") {
		t.Errorf("expected guidance about flags, got: %v", err)
	}
}

func TestPaveHookInstall(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	settingsDir := filepath.Join(t.TempDir(), ".claude")
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(settingsDir, "settings.json")

	// Write minimal settings.
	if err := os.WriteFile(settingsPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// We can't easily test the full hook install path because it reads
	// os.UserHomeDir(). Instead, test the source package helpers directly.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Verify pave-check command is registered.
	rootCmd.SetArgs([]string{"pave-check", "--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("pave-check help: %v", err)
	}
}
