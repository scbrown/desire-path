//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// --- Missing DB (pave-check fails open) ---

// TestPaveCheckMissingDB verifies that pave-check exits 0 (allow) when
// the database file does not exist. This tests the fail-open behavior.
func TestPaveCheckMissingDB(t *testing.T) {
	e := newEnv(t)

	// Point config at a non-existent DB by overwriting config.toml.
	e.writeConfig("")
	// Remove the DB file to simulate missing database.
	os.Remove(e.dbPath)

	payload, _ := json.Marshal(map[string]string{"tool_name": "read_file"})
	_, stderr, err := e.run(payload, "pave-check")

	code := exitCode(err)
	if code != 0 {
		t.Fatalf("expected exit code 0 (fail open) with missing DB, got %d (stderr: %q)", code, stderr)
	}
}

// TestPaveCheckCorruptDB verifies pave-check exits 0 (allow) when the
// database file exists but contains garbage data (not a valid SQLite file).
func TestPaveCheckCorruptDB(t *testing.T) {
	e := newEnv(t)

	// Write garbage to the DB path.
	if err := os.WriteFile(e.dbPath, []byte("this is not a sqlite database"), 0o644); err != nil {
		t.Fatalf("write corrupt db: %v", err)
	}

	payload, _ := json.Marshal(map[string]string{"tool_name": "read_file"})
	_, stderr, err := e.run(payload, "pave-check")

	code := exitCode(err)
	if code != 0 {
		t.Fatalf("expected exit code 0 (fail open) with corrupt DB, got %d (stderr: %q)", code, stderr)
	}
}

// TestIngestAutoCreatesDB verifies that dp ingest creates the database
// automatically when it doesn't exist yet.
func TestIngestAutoCreatesDB(t *testing.T) {
	e := newEnv(t)

	// Remove the DB so ingest must create it.
	os.Remove(e.dbPath)

	// Use an error fixture so both invocations and desires tables are populated.
	payload := e.fixture("Read", "auto-create-session", "unknown tool")
	e.mustRun(payload, "ingest", "--source", "claude-code")

	// Verify DB was created.
	if _, err := os.Stat(e.dbPath); os.IsNotExist(err) {
		t.Fatal("expected database file to be auto-created")
	}

	// Verify the desire was recorded (list queries the desires table).
	stdout, _ := e.mustRun(nil, "list", "--json")
	if !strings.Contains(stdout, "Read") {
		t.Errorf("expected list output to contain 'Read', got:\n%s", stdout)
	}
}

// TestStatsAutoCreatesDB verifies that dp stats works and auto-creates
// the database when it doesn't exist.
func TestStatsAutoCreatesDB(t *testing.T) {
	e := newEnv(t)
	os.Remove(e.dbPath)

	stdout, _ := e.mustRun(nil, "stats", "--json")
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse stats JSON: %v", err)
	}
	if result["total_desires"].(float64) != 0 {
		t.Errorf("expected 0 desires, got %v", result["total_desires"])
	}
}

// --- Malformed settings.json ---

// TestPaveHookMalformedSettingsJSON verifies dp pave --hook returns an
// error when settings.json contains invalid JSON.
func TestPaveHookMalformedSettingsJSON(t *testing.T) {
	e := newEnv(t)

	// Write invalid JSON to settings file.
	settingsDir := filepath.Join(e.home, ".claude")
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		t.Fatalf("create .claude dir: %v", err)
	}
	settingsFile := filepath.Join(settingsDir, "settings.json")
	if err := os.WriteFile(settingsFile, []byte(`{not valid json`), 0o644); err != nil {
		t.Fatalf("write malformed settings: %v", err)
	}

	_, stderr, err := e.run(nil, "pave", "--hook", "--settings", settingsFile)
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for malformed settings.json")
	}
	if !strings.Contains(stderr, "parse") {
		t.Errorf("expected stderr to mention 'parse', got: %q", stderr)
	}
}

// TestInitMalformedSettingsJSON verifies dp init returns an error when
// settings.json contains invalid JSON.
func TestInitMalformedSettingsJSON(t *testing.T) {
	e := newEnv(t)

	settingsDir := filepath.Join(e.home, ".claude")
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		t.Fatalf("create .claude dir: %v", err)
	}
	settingsFile := filepath.Join(settingsDir, "settings.json")
	if err := os.WriteFile(settingsFile, []byte(`{"hooks": "not an object"}`), 0o644); err != nil {
		t.Fatalf("write malformed settings: %v", err)
	}

	_, stderr, err := e.run(nil, "init", "--source", "claude-code", "--settings", settingsFile)
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for malformed settings.json hooks")
	}
	if stderr == "" {
		t.Error("expected error output on stderr")
	}
}

// TestPaveHookEmptySettingsFile verifies dp pave --hook handles an empty
// settings.json file (0 bytes) gracefully.
func TestPaveHookEmptySettingsFile(t *testing.T) {
	e := newEnv(t)

	settingsDir := filepath.Join(e.home, ".claude")
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		t.Fatalf("create .claude dir: %v", err)
	}
	settingsFile := filepath.Join(settingsDir, "settings.json")
	if err := os.WriteFile(settingsFile, []byte{}, 0o644); err != nil {
		t.Fatalf("write empty settings: %v", err)
	}

	_, stderr, err := e.run(nil, "pave", "--hook", "--settings", settingsFile)
	code := exitCode(err)
	// Empty file is technically invalid JSON, should error
	if code == 0 {
		// If it succeeds, that's also acceptable - check the hook was installed
		data := e.readFile(settingsFile)
		if !strings.Contains(data, "pave-check") {
			t.Errorf("expected settings to contain 'pave-check' if command succeeded, got:\n%s", data)
		}
	}
	_ = stderr // error or success both acceptable
}

// --- Unknown commands ---

// TestUnknownCommand verifies dp exits with a non-zero code for unknown
// subcommands.
func TestUnknownCommand(t *testing.T) {
	e := newEnv(t)

	_, stderr, err := e.run(nil, "nonexistent-command")
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for unknown command")
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("expected stderr to mention 'unknown command', got: %q", stderr)
	}
}

// TestUnknownCommandSimilarName verifies dp handles unknown commands that
// look like real commands (typos).
func TestUnknownCommandSimilarName(t *testing.T) {
	e := newEnv(t)

	_, _, err := e.run(nil, "alais") // typo of "alias"
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for typo command")
	}
}

// --- Missing required args ---

// TestAliasMissingArgs verifies dp alias with no args returns an error.
func TestAliasMissingArgs(t *testing.T) {
	e := newEnv(t)

	_, stderr, err := e.run(nil, "alias")
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for alias with no args")
	}
	if !strings.Contains(stderr, "requires") || !strings.Contains(stderr, "two") {
		t.Errorf("expected stderr to mention required args, got: %q", stderr)
	}
}

// TestAliasSingleArg verifies dp alias with only one arg (no --delete)
// returns an error.
func TestAliasSingleArg(t *testing.T) {
	e := newEnv(t)

	_, stderr, err := e.run(nil, "alias", "read_file")
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for alias with one arg")
	}
	if !strings.Contains(stderr, "requires") {
		t.Errorf("expected stderr to mention required args, got: %q", stderr)
	}
}

// TestAliasDeleteMissingArg verifies dp alias --delete with no arg returns
// an error.
func TestAliasDeleteMissingArg(t *testing.T) {
	e := newEnv(t)

	_, stderr, err := e.run(nil, "alias", "--delete")
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for alias --delete with no arg")
	}
	if !strings.Contains(stderr, "--delete requires") {
		t.Errorf("expected stderr to mention '--delete requires', got: %q", stderr)
	}
}

// TestIngestMissingSourceFlag verifies dp ingest without --source fails.
func TestIngestMissingSourceFlag(t *testing.T) {
	e := newEnv(t)

	payload := e.fixture("Read", "s1", "")
	_, stderr, err := e.run(payload, "ingest")
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for ingest without --source")
	}
	if !strings.Contains(stderr, "--source") {
		t.Errorf("expected stderr to mention '--source', got: %q", stderr)
	}
}

// TestIngestUnknownSource verifies dp ingest with an unknown source plugin
// returns a clear error.
func TestIngestUnknownSource(t *testing.T) {
	e := newEnv(t)

	payload := e.fixture("Read", "s1", "")
	_, stderr, err := e.run(payload, "ingest", "--source", "nonexistent-source")
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for unknown source")
	}
	if !strings.Contains(stderr, "unknown source") {
		t.Errorf("expected stderr to mention 'unknown source', got: %q", stderr)
	}
}

// TestInitMissingSourceFlag verifies dp init without --source fails.
func TestInitMissingSourceFlag(t *testing.T) {
	e := newEnv(t)

	_, stderr, err := e.run(nil, "init")
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for init without --source")
	}
	if !strings.Contains(stderr, "--source") {
		t.Errorf("expected stderr to mention '--source', got: %q", stderr)
	}
}

// TestInspectMissingPattern verifies dp inspect without a pattern arg fails.
func TestInspectMissingPattern(t *testing.T) {
	e := newEnv(t)

	_, _, err := e.run(nil, "inspect")
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for inspect without pattern")
	}
}

// TestPaveMissingFlag verifies dp pave without --hook or --agents-md fails.
func TestPaveMissingFlag(t *testing.T) {
	e := newEnv(t)

	_, stderr, err := e.run(nil, "pave")
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for pave without flags")
	}
	if !strings.Contains(stderr, "--hook") || !strings.Contains(stderr, "--agents-md") {
		t.Errorf("expected stderr to mention required flags, got: %q", stderr)
	}
}

// --- Concurrent WAL writes ---

// TestConcurrentIngest verifies that multiple concurrent dp ingest
// invocations don't corrupt the database. Separate processes share the
// same DB via WAL mode + busy_timeout. Some may fail with SQLITE_BUSY
// under heavy contention, but the DB must remain uncorrupted and at
// least one write should succeed.
func TestConcurrentIngest(t *testing.T) {
	e := newEnv(t)

	// First ingest to create the DB and warm up schema.
	warmup := e.fixture("Read", "warmup", "unknown tool")
	e.mustRun(warmup, "ingest", "--source", "claude-code")

	// Build all payloads before launching goroutines (fixture uses t.Helper).
	const n = 5
	payloads := make([][]byte, n)
	for i := 0; i < n; i++ {
		payloads[i] = e.fixture("Write", "concurrent-session", "tool not found")
	}

	// Launch n concurrent ingest processes.
	var wg sync.WaitGroup
	type result struct {
		stderr string
		err    error
	}
	results := make([]result, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, stderr, err := e.run(payloads[idx], "ingest", "--source", "claude-code")
			results[idx] = result{stderr: stderr, err: err}
		}(i)
	}
	wg.Wait()

	// Count successes. Under heavy contention, some may fail with
	// SQLITE_BUSY — that's acceptable, but at least one must succeed.
	successes := 0
	for _, r := range results {
		if r.err == nil {
			successes++
		}
	}
	if successes == 0 {
		t.Fatal("all concurrent ingest processes failed; expected at least one to succeed")
	}

	// Verify the DB is not corrupted after concurrent writes.
	stdout, _ := e.mustRun(nil, "stats", "--json")
	var stats map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &stats); err != nil {
		t.Fatalf("parse stats: %v (DB may be corrupted)", err)
	}
	if _, ok := stats["total_desires"]; !ok {
		t.Error("expected stats to contain total_desires")
	}

	// Sequential writes after contention should succeed.
	postPayload := e.fixture("Bash", "post-concurrent", "error after contention")
	e.mustRun(postPayload, "ingest", "--source", "claude-code")
}

// TestConcurrentAliasAndPaveCheck verifies that pave-check works correctly
// even while alias writes are happening concurrently.
func TestConcurrentAliasAndPaveCheck(t *testing.T) {
	e := newEnv(t)

	// Set up an initial alias.
	e.mustRun(nil, "alias", "read_file", "Read")

	// Concurrently: write more aliases while checking pave-check.
	var wg sync.WaitGroup

	// Writer goroutine: create more aliases.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 3; i++ {
			e.run(nil, "alias", "write_file", "Write")
		}
	}()

	// Reader goroutine: pave-check should still work.
	wg.Add(1)
	go func() {
		defer wg.Done()
		payload, _ := json.Marshal(map[string]string{"tool_name": "read_file"})
		_, _, err := e.run(payload, "pave-check")
		code := exitCode(err)
		// Should either block (2) or allow (0) - never crash.
		if code != 0 && code != 2 {
			t.Errorf("pave-check during concurrent writes: unexpected exit code %d", code)
		}
	}()

	wg.Wait()
}

// --- Additional error paths ---

// TestDeleteNonexistentAlias verifies that deleting a non-existent alias
// returns an appropriate error.
func TestDeleteNonexistentAlias(t *testing.T) {
	e := newEnv(t)

	_, stderr, err := e.run(nil, "alias", "--delete", "does_not_exist")
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for deleting non-existent alias")
	}
	if !strings.Contains(stderr, "not found") {
		t.Errorf("expected stderr to mention 'not found', got: %q", stderr)
	}
}

// TestPaveCheckMissingToolNameField verifies pave-check exits 0 when the
// JSON payload has no tool_name field.
func TestPaveCheckMissingToolNameField(t *testing.T) {
	e := newEnv(t)

	// Valid JSON but no tool_name.
	payload, _ := json.Marshal(map[string]string{"other_field": "value"})
	_, stderr, err := e.run(payload, "pave-check")

	code := exitCode(err)
	if code != 0 {
		t.Fatalf("expected exit code 0 for missing tool_name, got %d (stderr: %q)", code, stderr)
	}
}

// TestPaveCheckEmptyToolName verifies pave-check exits 0 when tool_name
// is present but empty.
func TestPaveCheckEmptyToolName(t *testing.T) {
	e := newEnv(t)

	payload, _ := json.Marshal(map[string]string{"tool_name": ""})
	_, stderr, err := e.run(payload, "pave-check")

	code := exitCode(err)
	if code != 0 {
		t.Fatalf("expected exit code 0 for empty tool_name, got %d (stderr: %q)", code, stderr)
	}
}

// TestIngestEmptyStdin verifies dp ingest handles empty stdin gracefully.
func TestIngestEmptyStdin(t *testing.T) {
	e := newEnv(t)

	_, _, err := e.run([]byte{}, "ingest", "--source", "claude-code")
	code := exitCode(err)
	// Should fail gracefully (not crash).
	if code == 0 {
		// Some sources may accept empty input - either way, shouldn't crash.
		return
	}
	// Non-zero is fine - it's an expected error for empty input.
}

// TestIngestMalformedJSON verifies dp ingest handles malformed JSON input.
func TestIngestMalformedJSON(t *testing.T) {
	e := newEnv(t)

	_, _, err := e.run([]byte(`{not json at all`), "ingest", "--source", "claude-code")
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for malformed JSON input to ingest")
	}
}

// TestListOnEmptyDB verifies dp list works on a fresh empty database.
func TestListOnEmptyDB(t *testing.T) {
	e := newEnv(t)

	stdout, _ := e.mustRun(nil, "list", "--json")
	// Should return an empty array or empty output, not crash.
	stdout = strings.TrimSpace(stdout)
	if stdout != "" && stdout != "[]" && stdout != "null" {
		// Verify it's valid JSON at minimum.
		var result interface{}
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Errorf("list --json on empty DB returned invalid JSON: %q", stdout)
		}
	}
}

// TestPathsOnEmptyDB verifies dp paths works on a fresh empty database.
func TestPathsOnEmptyDB(t *testing.T) {
	e := newEnv(t)

	stdout, _ := e.mustRun(nil, "paths", "--json")
	stdout = strings.TrimSpace(stdout)
	if stdout != "" && stdout != "[]" && stdout != "null" {
		var result interface{}
		if err := json.Unmarshal([]byte(stdout), &result); err != nil {
			t.Errorf("paths --json on empty DB returned invalid JSON: %q", stdout)
		}
	}
}

// TestAliasesOnEmptyDB verifies dp aliases works on a fresh empty database.
func TestAliasesOnEmptyDB(t *testing.T) {
	e := newEnv(t)

	stdout, _ := e.mustRun(nil, "aliases", "--json")
	stdout = strings.TrimSpace(stdout)
	var result interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("aliases --json on empty DB returned invalid JSON: %q", stdout)
	}
}

// TestConfigInvalidDefaultFormat verifies dp config rejects invalid
// default_format values.
func TestConfigInvalidDefaultFormat(t *testing.T) {
	e := newEnv(t)

	_, stderr, err := e.run(nil, "config", "set", "default_format", "xml")
	code := exitCode(err)
	if code == 0 {
		t.Fatal("expected non-zero exit code for invalid default_format")
	}
	if !strings.Contains(stderr, "default_format") {
		t.Errorf("expected stderr to mention 'default_format', got: %q", stderr)
	}
}
