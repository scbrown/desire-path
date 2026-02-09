package cli

// Tests for record/ingest convergence (dp-3u2).
//
// Covers:
//   1. dp record via source plugin produces identical Desire output as dp ingest
//   2. dp ingest dual-writes desire when is_error=true
//   3. dp record alias delegates to ingest path correctly
//   4. dp init installs single dp ingest hook for both events
//   5. Backward compat — old dp record hooks still work
//   6. Round-trip: failure payload through dp ingest appears in both
//      dp list and dp export --type invocations

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/config"
	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/source"
	"github.com/scbrown/desire-path/internal/store"
)

// --- Test 1: dp record via source plugin produces identical output as dp ingest ---

func TestConvergenceRecordAndIngestProduceIdenticalDesire(t *testing.T) {
	// Ingest a failure payload via "dp ingest" and via "dp record" with the
	// same input. Both must produce an invocation AND a desire with matching
	// field values (ignoring auto-generated IDs and timestamps).

	payload := `{"tool_name":"Bash","session_id":"conv-s1","cwd":"/home/user","error":"command not found","tool_input":{"command":"nonexistent"}}`

	// --- Run via dp ingest ---
	resetFlags(t)
	tmpIngest := t.TempDir()
	cfgIngest := filepath.Join(tmpIngest, "config.toml")
	dbIngest := filepath.Join(tmpIngest, "ingest.db")
	(&config.Config{}).SaveTo(cfgIngest)

	oldCfg, oldDB, oldJSON, oldStdin := configPath, dbPath, jsonOutput, os.Stdin
	configPath = cfgIngest
	dbPath = dbIngest
	jsonOutput = false
	defer func() {
		configPath = oldCfg
		dbPath = oldDB
		jsonOutput = oldJSON
		os.Stdin = oldStdin
	}()

	pipeStdin(t, payload)
	rootCmd.SetArgs([]string{"ingest", "--source", "claude-code"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("dp ingest: %v", err)
	}

	sIngest, err := store.New(dbIngest)
	if err != nil {
		t.Fatalf("open ingest db: %v", err)
	}
	defer sIngest.Close()

	ingestInvs, err := sIngest.ListInvocations(context.Background(), store.InvocationOpts{})
	if err != nil {
		t.Fatalf("list ingest invocations: %v", err)
	}
	ingestDesires, err := sIngest.ListDesires(context.Background(), store.ListOpts{})
	if err != nil {
		t.Fatalf("list ingest desires: %v", err)
	}

	// --- Run via dp record ---
	resetFlags(t)
	tmpRecord := t.TempDir()
	cfgRecord := filepath.Join(tmpRecord, "config.toml")
	dbRecord := filepath.Join(tmpRecord, "record.db")
	(&config.Config{}).SaveTo(cfgRecord)

	configPath = cfgRecord
	dbPath = dbRecord

	pipeStdin(t, payload)
	rootCmd.SetArgs([]string{"record", "--source", "claude-code"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("dp record: %v", err)
	}

	sRecord, err := store.New(dbRecord)
	if err != nil {
		t.Fatalf("open record db: %v", err)
	}
	defer sRecord.Close()

	recordInvs, err := sRecord.ListInvocations(context.Background(), store.InvocationOpts{})
	if err != nil {
		t.Fatalf("list record invocations: %v", err)
	}
	recordDesires, err := sRecord.ListDesires(context.Background(), store.ListOpts{})
	if err != nil {
		t.Fatalf("list record desires: %v", err)
	}

	// Both should produce exactly 1 invocation and 1 desire.
	if len(ingestInvs) != 1 {
		t.Fatalf("dp ingest: expected 1 invocation, got %d", len(ingestInvs))
	}
	if len(recordInvs) != 1 {
		t.Fatalf("dp record: expected 1 invocation, got %d", len(recordInvs))
	}
	if len(ingestDesires) != 1 {
		t.Fatalf("dp ingest: expected 1 desire, got %d", len(ingestDesires))
	}
	if len(recordDesires) != 1 {
		t.Fatalf("dp record: expected 1 desire, got %d", len(recordDesires))
	}

	// Compare invocation fields (excluding auto-generated ID/timestamp).
	iInv := ingestInvs[0]
	rInv := recordInvs[0]
	assertInvocationsEqual(t, iInv, rInv)

	// Compare desire fields (excluding auto-generated ID/timestamp).
	iDes := ingestDesires[0]
	rDes := recordDesires[0]
	assertDesiresEqual(t, iDes, rDes)
}

// --- Test 2: dp ingest dual-writes desire when is_error=true ---

func TestConvergenceIngestDualWritesDesireOnError(t *testing.T) {
	resetFlags(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	dbFile := filepath.Join(tmpDir, "test.db")
	(&config.Config{}).SaveTo(cfgPath)

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

	// Error payload: must produce both invocation and desire.
	pipeStdin(t, `{"tool_name":"Write","session_id":"dual-s1","cwd":"/tmp","error":"permission denied"}`)
	rootCmd.SetArgs([]string{"ingest", "--source", "claude-code"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ingest error: %v", err)
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
	if !invs[0].IsError {
		t.Error("invocation IsError should be true")
	}
	if invs[0].Error != "permission denied" {
		t.Errorf("invocation Error = %q, want %q", invs[0].Error, "permission denied")
	}

	desires, err := s.ListDesires(context.Background(), store.ListOpts{})
	if err != nil {
		t.Fatalf("list desires: %v", err)
	}
	if len(desires) != 1 {
		t.Fatalf("expected 1 desire (dual-write), got %d", len(desires))
	}
	if desires[0].ToolName != "Write" {
		t.Errorf("desire ToolName = %q, want %q", desires[0].ToolName, "Write")
	}
	if desires[0].Error != "permission denied" {
		t.Errorf("desire Error = %q, want %q", desires[0].Error, "permission denied")
	}
	if desires[0].Source != "claude-code" {
		t.Errorf("desire Source = %q, want %q", desires[0].Source, "claude-code")
	}
	if desires[0].SessionID != "dual-s1" {
		t.Errorf("desire SessionID = %q, want %q", desires[0].SessionID, "dual-s1")
	}
}

func TestConvergenceIngestNoDualWriteOnSuccess(t *testing.T) {
	resetFlags(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	dbFile := filepath.Join(tmpDir, "test.db")
	(&config.Config{}).SaveTo(cfgPath)

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

	// Success payload (no error): must produce only invocation, no desire.
	pipeStdin(t, `{"tool_name":"Read","session_id":"success-s1","cwd":"/tmp"}`)
	rootCmd.SetArgs([]string{"ingest", "--source", "claude-code"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ingest error: %v", err)
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
	if invs[0].IsError {
		t.Error("invocation IsError should be false for success")
	}

	desires, err := s.ListDesires(context.Background(), store.ListOpts{})
	if err != nil {
		t.Fatalf("list desires: %v", err)
	}
	if len(desires) != 0 {
		t.Errorf("expected 0 desires for success, got %d", len(desires))
	}
}

// --- Test 3: dp record alias delegates to ingest path correctly ---

func TestConvergenceRecordDelegatesToIngestPipeline(t *testing.T) {
	resetFlags(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	dbFile := filepath.Join(tmpDir, "test.db")
	(&config.Config{}).SaveTo(cfgPath)

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

	// dp record with an error payload should create both invocation and desire
	// via the ingest pipeline (not via any legacy record-only path).
	pipeStdin(t, `{"tool_name":"Grep","session_id":"rec-s1","cwd":"/var","error":"tool not found","tool_input":{"pattern":"foo"}}`)
	rootCmd.SetArgs([]string{"record", "--source", "claude-code"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("dp record: %v", err)
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
	inv := invs[0]
	if inv.ToolName != "Grep" {
		t.Errorf("invocation ToolName = %q, want %q", inv.ToolName, "Grep")
	}
	if inv.Source != "claude-code" {
		t.Errorf("invocation Source = %q, want %q", inv.Source, "claude-code")
	}
	if !inv.IsError {
		t.Error("invocation IsError should be true")
	}

	desires, err := s.ListDesires(context.Background(), store.ListOpts{})
	if err != nil {
		t.Fatalf("list desires: %v", err)
	}
	if len(desires) != 1 {
		t.Fatalf("expected 1 desire (dual-write via ingest), got %d", len(desires))
	}
	d := desires[0]
	if d.ToolName != "Grep" {
		t.Errorf("desire ToolName = %q, want %q", d.ToolName, "Grep")
	}
	if d.SessionID != "rec-s1" {
		t.Errorf("desire SessionID = %q, want %q", d.SessionID, "rec-s1")
	}
	if d.CWD != "/var" {
		t.Errorf("desire CWD = %q, want %q", d.CWD, "/var")
	}
	if d.Error != "tool not found" {
		t.Errorf("desire Error = %q, want %q", d.Error, "tool not found")
	}
}

// --- Test 4: dp init installs single dp ingest hook for both events ---

func TestConvergenceInitInstallsSingleIngestHookForBothEvents(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, ".claude", "settings.json")

	cc := source.Get("claude-code")
	if cc == nil {
		t.Fatal("claude-code source not registered")
	}

	installer, ok := cc.(source.Installer)
	if !ok {
		t.Fatal("claude-code should implement Installer")
	}

	if err := installer.Install(source.InstallOpts{SettingsPath: settingsPath}); err != nil {
		t.Fatalf("Install: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings: %v", err)
	}

	hooksRaw, ok := settings["hooks"]
	if !ok {
		t.Fatal("settings should contain hooks")
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooks); err != nil {
		t.Fatalf("parsing hooks: %v", err)
	}

	// Both events must have hooks.
	for _, event := range []string{"PostToolUse", "PostToolUseFailure"} {
		raw, ok := hooks[event]
		if !ok {
			t.Fatalf("hooks should contain %s", event)
		}

		var entries []struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
			} `json:"hooks"`
		}
		if err := json.Unmarshal(raw, &entries); err != nil {
			t.Fatalf("parsing %s: %v", event, err)
		}

		if len(entries) != 1 {
			t.Fatalf("%s: expected 1 hook entry, got %d", event, len(entries))
		}
		cmd := entries[0].Hooks[0].Command
		if cmd != "dp ingest --source claude-code" {
			t.Errorf("%s: command = %q, want %q", event, cmd, "dp ingest --source claude-code")
		}
	}

	// Verify both events use the SAME command (single entry point).
	var ptuEntries, ptufEntries []struct {
		Hooks []struct{ Command string } `json:"hooks"`
	}
	json.Unmarshal(hooks["PostToolUse"], &ptuEntries)
	json.Unmarshal(hooks["PostToolUseFailure"], &ptufEntries)

	if ptuEntries[0].Hooks[0].Command != ptufEntries[0].Hooks[0].Command {
		t.Error("PostToolUse and PostToolUseFailure should use the same dp ingest command")
	}
}

// --- Test 5: Backward compat — old dp record hooks still work ---

func TestConvergenceBackwardCompatLegacyRecordHooksDetected(t *testing.T) {
	// Verify that IsInstalled detects legacy "dp record" hooks.
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	legacySettings := `{
  "hooks": {
    "PostToolUseFailure": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "dp record --source claude-code",
            "timeout": 5000
          }
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(legacySettings), 0o644); err != nil {
		t.Fatal(err)
	}

	cc := source.Get("claude-code")
	if cc == nil {
		t.Fatal("claude-code not registered")
	}
	installer, ok := cc.(source.Installer)
	if !ok {
		t.Fatal("claude-code should implement Installer")
	}

	installed, err := installer.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled: %v", err)
	}
	if !installed {
		t.Error("IsInstalled should detect legacy dp record hooks as installed")
	}
}

func TestConvergenceBackwardCompatRecordCommandStillWorks(t *testing.T) {
	// Verify that "dp record --source claude-code" still functions correctly.
	resetFlags(t)
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	dbFile := filepath.Join(tmpDir, "test.db")
	(&config.Config{}).SaveTo(cfgPath)

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

	pipeStdin(t, `{"tool_name":"Edit","session_id":"compat-s1","cwd":"/home","error":"file not found"}`)
	rootCmd.SetArgs([]string{"record", "--source", "claude-code"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("dp record: %v", err)
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
	if invs[0].ToolName != "Edit" {
		t.Errorf("ToolName = %q, want %q", invs[0].ToolName, "Edit")
	}

	desires, err := s.ListDesires(context.Background(), store.ListOpts{})
	if err != nil {
		t.Fatalf("list desires: %v", err)
	}
	if len(desires) != 1 {
		t.Fatalf("expected 1 desire, got %d", len(desires))
	}
	if desires[0].ToolName != "Edit" {
		t.Errorf("desire ToolName = %q, want %q", desires[0].ToolName, "Edit")
	}
}

func TestConvergenceBackwardCompatRecordRequiresSource(t *testing.T) {
	resetFlags(t)
	rootCmd.SetArgs([]string{"record"})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when --source not provided")
	}
	if !strings.Contains(err.Error(), "--source") {
		t.Errorf("error should mention --source, got: %v", err)
	}
}

// --- Test 6: Round-trip: failure through dp ingest appears in dp list and dp export ---

func TestConvergenceRoundTripFailureInListAndExport(t *testing.T) {
	resetFlags(t)
	t.Cleanup(func() { resetFlags(t) })
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	dbFile := filepath.Join(tmpDir, "roundtrip.db")
	(&config.Config{}).SaveTo(cfgPath)

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

	// Ingest a failure payload.
	pipeStdin(t, `{"tool_name":"Glob","session_id":"rt-s1","cwd":"/srv","error":"no matches found","tool_input":{"pattern":"*.xyz"}}`)
	rootCmd.SetArgs([]string{"ingest", "--source", "claude-code"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// Verify via dp list (JSON mode).
	resetFlags(t)
	configPath = cfgPath
	dbPath = dbFile
	jsonOutput = true

	listOutput := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"list", "--db", dbFile, "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("list: %v", err)
		}
	})

	var desires []model.Desire
	if err := json.Unmarshal([]byte(listOutput), &desires); err != nil {
		t.Fatalf("parse list output: %v\nraw: %s", err, listOutput)
	}
	if len(desires) != 1 {
		t.Fatalf("dp list: expected 1 desire, got %d", len(desires))
	}
	if desires[0].ToolName != "Glob" {
		t.Errorf("dp list desire ToolName = %q, want %q", desires[0].ToolName, "Glob")
	}
	if desires[0].Error != "no matches found" {
		t.Errorf("dp list desire Error = %q, want %q", desires[0].Error, "no matches found")
	}
	if desires[0].Source != "claude-code" {
		t.Errorf("dp list desire Source = %q, want %q", desires[0].Source, "claude-code")
	}

	// Verify via dp export --type invocations.
	resetFlags(t)
	configPath = cfgPath
	dbPath = dbFile

	exportOutput := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"export", "--db", dbFile, "--format", "json", "--type", "invocations"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("export: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(exportOutput), "\n")
	if len(lines) != 1 {
		t.Fatalf("dp export: expected 1 invocation line, got %d: %s", len(lines), exportOutput)
	}

	var inv model.Invocation
	if err := json.Unmarshal([]byte(lines[0]), &inv); err != nil {
		t.Fatalf("parse export invocation: %v", err)
	}
	if inv.ToolName != "Glob" {
		t.Errorf("dp export invocation ToolName = %q, want %q", inv.ToolName, "Glob")
	}
	if !inv.IsError {
		t.Error("dp export invocation IsError should be true")
	}
	if inv.Error != "no matches found" {
		t.Errorf("dp export invocation Error = %q, want %q", inv.Error, "no matches found")
	}
	if inv.Source != "claude-code" {
		t.Errorf("dp export invocation Source = %q, want %q", inv.Source, "claude-code")
	}
}

func TestConvergenceRoundTripSuccessInExportNotInList(t *testing.T) {
	// A successful invocation should appear in dp export --type invocations
	// but NOT in dp list (which only shows desires/failures).
	resetFlags(t)
	t.Cleanup(func() { resetFlags(t) })
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.toml")
	dbFile := filepath.Join(tmpDir, "roundtrip-success.db")
	(&config.Config{}).SaveTo(cfgPath)

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

	// Ingest a success payload (no error).
	pipeStdin(t, `{"tool_name":"Read","session_id":"rt-s2","cwd":"/tmp"}`)
	rootCmd.SetArgs([]string{"ingest", "--source", "claude-code"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	// dp list (desires) should be empty.
	resetFlags(t)
	configPath = cfgPath
	dbPath = dbFile
	jsonOutput = true

	listOutput := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"list", "--db", dbFile, "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("list: %v", err)
		}
	})

	var desires []model.Desire
	if err := json.Unmarshal([]byte(listOutput), &desires); err != nil {
		t.Fatalf("parse list: %v\nraw: %s", err, listOutput)
	}
	if len(desires) != 0 {
		t.Errorf("dp list: expected 0 desires for success, got %d", len(desires))
	}

	// dp export --type invocations should show the invocation.
	resetFlags(t)
	configPath = cfgPath
	dbPath = dbFile

	exportOutput := captureStdout(t, func() {
		rootCmd.SetArgs([]string{"export", "--db", dbFile, "--format", "json", "--type", "invocations"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("export: %v", err)
		}
	})

	lines := strings.Split(strings.TrimSpace(exportOutput), "\n")
	if len(lines) != 1 {
		t.Fatalf("dp export: expected 1 invocation, got %d", len(lines))
	}
	var inv model.Invocation
	if err := json.Unmarshal([]byte(lines[0]), &inv); err != nil {
		t.Fatalf("parse export: %v", err)
	}
	if inv.ToolName != "Read" {
		t.Errorf("invocation ToolName = %q, want %q", inv.ToolName, "Read")
	}
	if inv.IsError {
		t.Error("invocation IsError should be false for success")
	}
}

// --- Helpers ---

func assertInvocationsEqual(t *testing.T, a, b model.Invocation) {
	t.Helper()
	if a.ToolName != b.ToolName {
		t.Errorf("ToolName: %q vs %q", a.ToolName, b.ToolName)
	}
	if a.Source != b.Source {
		t.Errorf("Source: %q vs %q", a.Source, b.Source)
	}
	if a.InstanceID != b.InstanceID {
		t.Errorf("InstanceID: %q vs %q", a.InstanceID, b.InstanceID)
	}
	if a.IsError != b.IsError {
		t.Errorf("IsError: %v vs %v", a.IsError, b.IsError)
	}
	if a.Error != b.Error {
		t.Errorf("Error: %q vs %q", a.Error, b.Error)
	}
	if a.CWD != b.CWD {
		t.Errorf("CWD: %q vs %q", a.CWD, b.CWD)
	}
}

func assertDesiresEqual(t *testing.T, a, b model.Desire) {
	t.Helper()
	if a.ToolName != b.ToolName {
		t.Errorf("ToolName: %q vs %q", a.ToolName, b.ToolName)
	}
	if a.Error != b.Error {
		t.Errorf("Error: %q vs %q", a.Error, b.Error)
	}
	if a.Source != b.Source {
		t.Errorf("Source: %q vs %q", a.Source, b.Source)
	}
	if a.SessionID != b.SessionID {
		t.Errorf("SessionID: %q vs %q", a.SessionID, b.SessionID)
	}
	if a.CWD != b.CWD {
		t.Errorf("CWD: %q vs %q", a.CWD, b.CWD)
	}
}
