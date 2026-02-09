package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/config"
	"github.com/scbrown/desire-path/internal/store"
)

// pipeStdin replaces os.Stdin with a pipe fed by data.
// The caller must restore os.Stdin (typically via defer).
func pipeStdin(t *testing.T, data string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	go func() {
		defer w.Close()
		w.WriteString(data)
	}()
}

func TestIngestSkipsUnlistedTools(t *testing.T) {
	resetFlags(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	dbFile := filepath.Join(tmpDir, "test.db")

	// Allowlist permits Bash and Write only — "Read" should be skipped.
	cfg := &config.Config{TrackTools: []string{"Bash", "Write"}}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}

	oldCfg, oldDB, oldJSON, oldStdin := configPath, dbPath, jsonOutput, os.Stdin
	configPath = cfgPath
	dbPath = dbFile
	jsonOutput = true
	defer func() {
		configPath = oldCfg
		dbPath = oldDB
		jsonOutput = oldJSON
		os.Stdin = oldStdin
	}()

	// Claude Code payload with tool_name "Read" (NOT in allowlist).
	pipeStdin(t, `{"tool_name":"Read","session_id":"s1","cwd":"/tmp"}`)

	// Capture stdout to verify silence.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"ingest", "--source", "claude-code"})
	err := rootCmd.Execute()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("expected nil error (silent skip), got: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "" {
		t.Errorf("expected no output for skipped tool, got: %s", buf.String())
	}

	// Database should not have been created — command returned before opening it.
	if _, statErr := os.Stat(dbFile); statErr == nil {
		t.Error("database should not have been created for skipped tool")
	}
}

func TestIngestRecordsListedTools(t *testing.T) {
	resetFlags(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	dbFile := filepath.Join(tmpDir, "test.db")

	// Allowlist includes "Read".
	cfg := &config.Config{TrackTools: []string{"Read", "Bash"}}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}

	oldCfg, oldDB, oldJSON, oldStdin := configPath, dbPath, jsonOutput, os.Stdin
	configPath = cfgPath
	dbPath = dbFile
	jsonOutput = false
	defer func() {
		configPath = oldCfg
		dbPath = oldDB
		jsonOutput = oldJSON
		os.Stdin = oldStdin
	}()

	pipeStdin(t, `{"tool_name":"Read","session_id":"s2","cwd":"/tmp"}`)

	rootCmd.SetArgs([]string{"ingest", "--source", "claude-code"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify invocation was persisted.
	s, err := store.New(dbFile)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer s.Close()

	invs, err := s.ListInvocations(context.Background(), store.InvocationOpts{})
	if err != nil {
		t.Fatalf("list invocations: %v", err)
	}
	if len(invs) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(invs))
	}
	if invs[0].ToolName != "Read" {
		t.Errorf("tool_name: got %q, want %q", invs[0].ToolName, "Read")
	}
}

func TestIngestEmptyAllowlistTracksEverything(t *testing.T) {
	resetFlags(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	dbFile := filepath.Join(tmpDir, "test.db")

	// No TrackTools — everything should be recorded.
	cfg := &config.Config{}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}

	oldCfg, oldDB, oldJSON, oldStdin := configPath, dbPath, jsonOutput, os.Stdin
	configPath = cfgPath
	dbPath = dbFile
	jsonOutput = false
	defer func() {
		configPath = oldCfg
		dbPath = oldDB
		jsonOutput = oldJSON
		os.Stdin = oldStdin
	}()

	pipeStdin(t, `{"tool_name":"AnyTool","session_id":"s3","cwd":"/tmp"}`)

	rootCmd.SetArgs([]string{"ingest", "--source", "claude-code"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s, err := store.New(dbFile)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer s.Close()

	invs, err := s.ListInvocations(context.Background(), store.InvocationOpts{})
	if err != nil {
		t.Fatalf("list invocations: %v", err)
	}
	if len(invs) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(invs))
	}
	if invs[0].ToolName != "AnyTool" {
		t.Errorf("tool_name: got %q, want %q", invs[0].ToolName, "AnyTool")
	}
}

func TestRecordWithSourceDelegatesToIngest(t *testing.T) {
	resetFlags(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	dbFile := filepath.Join(tmpDir, "test.db")

	cfg := &config.Config{}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}

	oldCfg, oldDB, oldJSON, oldStdin := configPath, dbPath, jsonOutput, os.Stdin
	configPath = cfgPath
	dbPath = dbFile
	jsonOutput = false
	defer func() {
		configPath = oldCfg
		dbPath = oldDB
		jsonOutput = oldJSON
		os.Stdin = oldStdin
	}()

	pipeStdin(t, `{"tool_name":"Bash","session_id":"s10","cwd":"/tmp","error":"command failed"}`)

	rootCmd.SetArgs([]string{"record", "--source", "claude-code"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s, err := store.New(dbFile)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer s.Close()

	// Should create an invocation (via ingest pipeline).
	invs, err := s.ListInvocations(context.Background(), store.InvocationOpts{})
	if err != nil {
		t.Fatalf("list invocations: %v", err)
	}
	if len(invs) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(invs))
	}
	if invs[0].ToolName != "Bash" {
		t.Errorf("tool_name: got %q, want %q", invs[0].ToolName, "Bash")
	}
	if invs[0].Source != "claude-code" {
		t.Errorf("source: got %q, want %q", invs[0].Source, "claude-code")
	}

	// Error invocations also create a desire via dual-write.
	desires, err := s.ListDesires(context.Background(), store.ListOpts{})
	if err != nil {
		t.Fatalf("list desires: %v", err)
	}
	if len(desires) != 1 {
		t.Fatalf("expected 1 desire (dual-write for error), got %d", len(desires))
	}
	if desires[0].ToolName != "Bash" {
		t.Errorf("desire tool_name: got %q, want %q", desires[0].ToolName, "Bash")
	}
}

func TestRecordWithSourceRespectsAllowlist(t *testing.T) {
	resetFlags(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	dbFile := filepath.Join(tmpDir, "test.db")

	// Allowlist permits only "Read" — "Bash" should be skipped.
	cfg := &config.Config{TrackTools: []string{"Read"}}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}

	oldCfg, oldDB, oldJSON, oldStdin := configPath, dbPath, jsonOutput, os.Stdin
	configPath = cfgPath
	dbPath = dbFile
	jsonOutput = true
	defer func() {
		configPath = oldCfg
		dbPath = oldDB
		jsonOutput = oldJSON
		os.Stdin = oldStdin
	}()

	pipeStdin(t, `{"tool_name":"Bash","session_id":"s11","cwd":"/tmp"}`)

	// Capture stdout to verify silence.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"record", "--source", "claude-code"})
	err := rootCmd.Execute()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("expected nil error (silent skip), got: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "" {
		t.Errorf("expected no output for skipped tool, got: %s", buf.String())
	}

	// Database should not have been created.
	if _, statErr := os.Stat(dbFile); statErr == nil {
		t.Error("database should not have been created for skipped tool")
	}
}

func TestRecordRespectsAllowlistSameAsIngest(t *testing.T) {
	resetFlags(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	dbFile := filepath.Join(tmpDir, "test.db")

	// Allowlist restricts to Read and Bash only.
	// "Write" is NOT in the allowlist, so dp record (now an alias
	// for dp ingest) should also skip it.
	cfg := &config.Config{TrackTools: []string{"Read", "Bash"}}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}

	oldCfg, oldDB, oldJSON, oldStdin := configPath, dbPath, jsonOutput, os.Stdin
	configPath = cfgPath
	dbPath = dbFile
	jsonOutput = true
	defer func() {
		configPath = oldCfg
		dbPath = oldDB
		jsonOutput = oldJSON
		os.Stdin = oldStdin
	}()

	pipeStdin(t, `{"tool_name":"Write","error":"unknown tool"}`)

	// Capture stdout to verify silence.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"record", "--source", "claude-code"})
	err := rootCmd.Execute()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)

	if err != nil {
		t.Fatalf("expected nil error (silent skip), got: %v", err)
	}
	if strings.TrimSpace(buf.String()) != "" {
		t.Errorf("expected no output for skipped tool, got: %s", buf.String())
	}

	// Database should not have been created.
	if _, statErr := os.Stat(dbFile); statErr == nil {
		t.Error("database should not have been created for skipped tool")
	}
}
