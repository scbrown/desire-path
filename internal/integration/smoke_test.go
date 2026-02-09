//go:build integration

package integration

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSmokeDPVersion verifies the dp binary runs and prints version/help info.
func TestSmokeDPVersion(t *testing.T) {
	e := newEnv(t)
	stdout, _ := e.mustRun(nil, "--help")
	if !strings.Contains(stdout, "desire") {
		t.Errorf("expected help to mention 'desire', got:\n%s", stdout)
	}
}

// TestSmokeIngestAndList ingests a single desire and lists it back.
func TestSmokeIngestAndList(t *testing.T) {
	e := newEnv(t)

	// Ingest a tool call.
	payload := e.fixture("read_file", "test-session", "unknown tool")
	e.mustRun(payload, "ingest", "--source", "claude-code")

	// List desires as JSON.
	stdout, _ := e.mustRun(nil, "list", "--json")
	if !strings.Contains(stdout, "read_file") {
		t.Errorf("expected list output to contain 'read_file', got:\n%s", stdout)
	}
}

// TestSmokeStats verifies dp stats works on a fresh database.
func TestSmokeStats(t *testing.T) {
	e := newEnv(t)

	stdout, _ := e.mustRun(nil, "stats", "--json")

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse stats JSON: %v", err)
	}

	desires, ok := result["total_desires"]
	if !ok {
		t.Fatal("expected total_desires in stats output")
	}
	if desires.(float64) != 0 {
		t.Errorf("expected 0 desires on fresh db, got %v", desires)
	}
}

// TestSmokeInitClaudeCode verifies dp init creates hooks in settings.
func TestSmokeInitClaudeCode(t *testing.T) {
	e := newEnv(t)

	stdout, _ := e.mustRun(nil, "init", "--source", "claude-code",
		"--settings", e.settingsPath())

	if !strings.Contains(stdout, "configured") {
		t.Errorf("expected 'configured' in output, got:\n%s", stdout)
	}

	// Verify settings file was created.
	data := e.readFile(e.settingsPath())
	if !strings.Contains(data, "dp ingest") {
		t.Errorf("expected settings to contain 'dp ingest', got:\n%s", data)
	}
}

// TestSmokePaveHook verifies dp pave --hook installs the PreToolUse hook.
func TestSmokePaveHook(t *testing.T) {
	e := newEnv(t)

	// Pre-create empty settings so pave --hook has something to merge into.
	e.writeSettings(map[string]interface{}{})

	stdout, _ := e.mustRun(nil, "pave", "--hook",
		"--settings", e.settingsPath())

	if !strings.Contains(stdout, "installed") && !strings.Contains(stdout, "configured") {
		t.Errorf("expected 'installed' or 'configured' in output, got:\n%s", stdout)
	}

	// Verify the hook is present.
	data := e.readFile(e.settingsPath())
	if !strings.Contains(data, "pave-check") {
		t.Errorf("expected settings to contain 'pave-check', got:\n%s", data)
	}
}
