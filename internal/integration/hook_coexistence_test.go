//go:build integration

package integration

import (
	"encoding/json"
	"testing"
)

// TestBothHooksInstall verifies that running dp init followed by dp pave --hook
// results in settings.json containing all three hook events without clobbering.
func TestBothHooksInstall(t *testing.T) {
	e := newEnv(t)

	// Install init hooks first (PostToolUse + PostToolUseFailure).
	e.mustRun(nil, "init", "--source", "claude-code", "--settings", e.settingsPath())

	// Install pave hook second (PreToolUse).
	e.mustRun(nil, "pave", "--hook", "--settings", e.settingsPath())

	// Parse and verify all three hook events are present.
	settings := e.readSettingsJSON()
	hooks := settingsHooks(t, settings)

	assertHookCommand(t, hooks, "PostToolUse", "dp ingest --source claude-code")
	assertHookCommand(t, hooks, "PostToolUseFailure", "dp ingest --source claude-code")
	assertHookCommand(t, hooks, "PreToolUse", "dp pave-check")
}

// TestBothHooksInstallReverseOrder verifies installation works in either order.
func TestBothHooksInstallReverseOrder(t *testing.T) {
	e := newEnv(t)

	// Pave first, then init.
	e.writeSettings(map[string]interface{}{})
	e.mustRun(nil, "pave", "--hook", "--settings", e.settingsPath())
	e.mustRun(nil, "init", "--source", "claude-code", "--settings", e.settingsPath())

	settings := e.readSettingsJSON()
	hooks := settingsHooks(t, settings)

	assertHookCommand(t, hooks, "PostToolUse", "dp ingest --source claude-code")
	assertHookCommand(t, hooks, "PostToolUseFailure", "dp ingest --source claude-code")
	assertHookCommand(t, hooks, "PreToolUse", "dp pave-check")
}

// TestInitIdempotent verifies running dp init twice does not duplicate hooks.
func TestInitIdempotent(t *testing.T) {
	e := newEnv(t)

	e.mustRun(nil, "init", "--source", "claude-code", "--settings", e.settingsPath())
	e.mustRun(nil, "init", "--source", "claude-code", "--settings", e.settingsPath())

	settings := e.readSettingsJSON()
	hooks := settingsHooks(t, settings)

	assertHookCount(t, hooks, "PostToolUse", 1)
	assertHookCount(t, hooks, "PostToolUseFailure", 1)
}

// TestPaveHookIdempotent verifies running dp pave --hook twice does not
// duplicate the PreToolUse hook.
func TestPaveHookIdempotent(t *testing.T) {
	e := newEnv(t)
	e.writeSettings(map[string]interface{}{})

	e.mustRun(nil, "pave", "--hook", "--settings", e.settingsPath())
	e.mustRun(nil, "pave", "--hook", "--settings", e.settingsPath())

	settings := e.readSettingsJSON()
	hooks := settingsHooks(t, settings)

	assertHookCount(t, hooks, "PreToolUse", 1)
}

// TestBothIdempotent verifies running both commands multiple times in various
// orders never produces duplicate entries.
func TestBothIdempotent(t *testing.T) {
	e := newEnv(t)

	// Run each command three times in mixed order.
	e.mustRun(nil, "init", "--source", "claude-code", "--settings", e.settingsPath())
	e.mustRun(nil, "pave", "--hook", "--settings", e.settingsPath())
	e.mustRun(nil, "init", "--source", "claude-code", "--settings", e.settingsPath())
	e.mustRun(nil, "pave", "--hook", "--settings", e.settingsPath())
	e.mustRun(nil, "init", "--source", "claude-code", "--settings", e.settingsPath())
	e.mustRun(nil, "pave", "--hook", "--settings", e.settingsPath())

	settings := e.readSettingsJSON()
	hooks := settingsHooks(t, settings)

	assertHookCount(t, hooks, "PostToolUse", 1)
	assertHookCount(t, hooks, "PostToolUseFailure", 1)
	assertHookCount(t, hooks, "PreToolUse", 1)
}

// TestPreserveUserHooks verifies that existing user-defined hooks are not
// clobbered when dp installs its hooks.
func TestPreserveUserHooks(t *testing.T) {
	e := newEnv(t)

	// Pre-seed settings with user-defined hooks on the same events dp uses.
	e.writeSettings(map[string]interface{}{
		"hooks": map[string]interface{}{
			"PostToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Bash",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "my-custom-logger",
							"timeout": 2000,
						},
					},
				},
			},
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"matcher": "Write",
					"hooks": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": "my-linter",
							"timeout": 1000,
						},
					},
				},
			},
		},
	})

	// Install both dp hooks.
	e.mustRun(nil, "init", "--source", "claude-code", "--settings", e.settingsPath())
	e.mustRun(nil, "pave", "--hook", "--settings", e.settingsPath())

	settings := e.readSettingsJSON()
	hooks := settingsHooks(t, settings)

	// User hooks must still exist.
	assertHookCommand(t, hooks, "PostToolUse", "my-custom-logger")
	assertHookCommand(t, hooks, "PreToolUse", "my-linter")

	// DP hooks must also exist.
	assertHookCommand(t, hooks, "PostToolUse", "dp ingest --source claude-code")
	assertHookCommand(t, hooks, "PostToolUseFailure", "dp ingest --source claude-code")
	assertHookCommand(t, hooks, "PreToolUse", "dp pave-check")

	// PostToolUse should have 2 entries (user + dp), PreToolUse should have 2.
	assertHookCount(t, hooks, "PostToolUse", 2)
	assertHookCount(t, hooks, "PreToolUse", 2)
}

// TestPreserveNonHookSettings verifies that non-hook settings keys are
// preserved when dp installs hooks.
func TestPreserveNonHookSettings(t *testing.T) {
	e := newEnv(t)

	// Pre-seed with a variety of non-hook settings.
	e.writeSettings(map[string]interface{}{
		"permissions": map[string]interface{}{
			"allow": []string{"Read", "Write", "Bash"},
		},
		"env": map[string]interface{}{
			"MY_VAR":   "hello",
			"API_MODE": "test",
		},
		"model":             "claude-sonnet-4-5-20250929",
		"customInstructions": "Be helpful and concise.",
	})

	// Install both hooks.
	e.mustRun(nil, "init", "--source", "claude-code", "--settings", e.settingsPath())
	e.mustRun(nil, "pave", "--hook", "--settings", e.settingsPath())

	settings := e.readSettingsJSON()

	// All original keys must still be present.
	for _, key := range []string{"permissions", "env", "model", "customInstructions"} {
		if _, ok := settings[key]; !ok {
			t.Errorf("settings key %q was lost after hook installation", key)
		}
	}

	// Verify specific values survived.
	var model string
	if err := json.Unmarshal(settings["model"], &model); err != nil {
		t.Fatalf("unmarshal model: %v", err)
	}
	if model != "claude-sonnet-4-5-20250929" {
		t.Errorf("model = %q, want claude-sonnet-4-5-20250929", model)
	}

	var instructions string
	if err := json.Unmarshal(settings["customInstructions"], &instructions); err != nil {
		t.Fatalf("unmarshal customInstructions: %v", err)
	}
	if instructions != "Be helpful and concise." {
		t.Errorf("customInstructions = %q, want 'Be helpful and concise.'", instructions)
	}

	// Hooks must also be present.
	hooks := settingsHooks(t, settings)
	assertHookCommand(t, hooks, "PostToolUse", "dp ingest --source claude-code")
	assertHookCommand(t, hooks, "PreToolUse", "dp pave-check")
}

// TestCreateFromScratch verifies that dp init and dp pave --hook work when
// no settings.json exists at all (creates parent directories too).
func TestCreateFromScratch(t *testing.T) {
	e := newEnv(t)

	// dp init should create the file from nothing.
	e.mustRun(nil, "init", "--source", "claude-code", "--settings", e.settingsPath())

	settings := e.readSettingsJSON()
	hooks := settingsHooks(t, settings)
	assertHookCommand(t, hooks, "PostToolUse", "dp ingest --source claude-code")
	assertHookCommand(t, hooks, "PostToolUseFailure", "dp ingest --source claude-code")
}

// TestCreateFromScratchPave verifies dp pave --hook creates settings.json
// from nothing.
func TestCreateFromScratchPave(t *testing.T) {
	e := newEnv(t)

	// pave --hook should create the file from nothing.
	e.mustRun(nil, "pave", "--hook", "--settings", e.settingsPath())

	settings := e.readSettingsJSON()
	hooks := settingsHooks(t, settings)
	assertHookCommand(t, hooks, "PreToolUse", "dp pave-check")
}

// TestInitAlreadyConfiguredOutput verifies dp init reports "already_configured"
// when hooks are already present, rather than duplicating.
func TestInitAlreadyConfiguredOutput(t *testing.T) {
	e := newEnv(t)

	// First install.
	e.mustRun(nil, "init", "--source", "claude-code", "--settings", e.settingsPath())

	// Second install should say already configured.
	stdout, _ := e.mustRun(nil, "init", "--source", "claude-code", "--settings", e.settingsPath(), "--json")

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse JSON output: %v", err)
	}
	status, _ := result["status"].(string)
	if status != "already_configured" {
		t.Errorf("status = %q, want already_configured", status)
	}
}

// TestPaveHookAlreadyConfiguredOutput verifies dp pave --hook reports
// "already_configured" when the hook is already present.
func TestPaveHookAlreadyConfiguredOutput(t *testing.T) {
	e := newEnv(t)
	e.writeSettings(map[string]interface{}{})

	// First install.
	e.mustRun(nil, "pave", "--hook", "--settings", e.settingsPath())

	// Second install should say already configured.
	stdout, _ := e.mustRun(nil, "pave", "--hook", "--settings", e.settingsPath(), "--json")

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse JSON output: %v", err)
	}
	status, _ := result["status"].(string)
	if status != "already_configured" {
		t.Errorf("status = %q, want already_configured", status)
	}
}

// --- helpers ---

// readSettingsJSON reads and parses the settings.json from this test env.
func (e *dpEnv) readSettingsJSON() map[string]json.RawMessage {
	e.t.Helper()
	data := e.readFile(e.settingsPath())
	var s map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &s); err != nil {
		e.t.Fatalf("parse settings.json: %v", err)
	}
	return s
}

// settingsHooks extracts the "hooks" map from parsed settings.
func settingsHooks(t *testing.T, settings map[string]json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	raw, ok := settings["hooks"]
	if !ok {
		t.Fatal("settings has no 'hooks' key")
	}
	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(raw, &hooks); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}
	return hooks
}

// hookEntry mirrors the Claude Code hook structure for test assertions.
type hookEntry struct {
	Matcher string      `json:"matcher"`
	Hooks   []hookInner `json:"hooks"`
}

type hookInner struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// assertHookCommand verifies that a specific command exists in the named
// hook event's entries.
func assertHookCommand(t *testing.T, hooks map[string]json.RawMessage, event, command string) {
	t.Helper()
	raw, ok := hooks[event]
	if !ok {
		t.Errorf("hook event %q not found", event)
		return
	}
	var entries []hookEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("parse %s entries: %v", event, err)
	}
	for _, e := range entries {
		for _, h := range e.Hooks {
			if h.Command == command {
				return
			}
		}
	}
	t.Errorf("command %q not found in %s hooks", command, event)
}

// assertHookCount verifies the number of hook entries for a given event.
func assertHookCount(t *testing.T, hooks map[string]json.RawMessage, event string, want int) {
	t.Helper()
	raw, ok := hooks[event]
	if !ok {
		if want == 0 {
			return
		}
		t.Errorf("hook event %q not found (want %d entries)", event, want)
		return
	}
	var entries []hookEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("parse %s entries: %v", event, err)
	}
	if len(entries) != want {
		t.Errorf("%s hook count = %d, want %d", event, len(entries), want)
	}
}
