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

	assertCommandCount(t, hooks, "PostToolUse", "dp ingest --source claude-code", 1)
	assertCommandCount(t, hooks, "PostToolUseFailure", "dp ingest --source claude-code", 1)
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

	assertCommandCount(t, hooks, "PreToolUse", "dp pave-check", 1)
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

	// Every command dp installs, exactly once each — across six interleaved
	// runs. Naming them individually is the point: the old form asserted the
	// number of groups, which could not tell "installed twice" from "two
	// different hooks live here".
	assertCommandCount(t, hooks, "PostToolUse", "dp ingest --source claude-code", 1)
	assertCommandCount(t, hooks, "PostToolUse", "dp signpost", 1)
	assertCommandCount(t, hooks, "PostToolUseFailure", "dp ingest --source claude-code", 1)
	assertCommandCount(t, hooks, "PostToolUseFailure", "dp pave-correct", 1)
	assertCommandCount(t, hooks, "PreToolUse", "dp pave-check", 1)
	assertCommandCount(t, hooks, "PreToolUse", "dp signpost-prefetch", 1)
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

	// The user's hook survives exactly once — not dropped, and not duplicated
	// by a second install pass. Asserting the group count instead only ever
	// tested this by coincidence, and stopped doing so when dp gained a second
	// hook on each of these events.
	assertCommandCount(t, hooks, "PostToolUse", "my-custom-logger", 1)
	assertCommandCount(t, hooks, "PreToolUse", "my-linter", 1)
	assertCommandCount(t, hooks, "PostToolUse", "dp ingest --source claude-code", 1)
	assertCommandCount(t, hooks, "PreToolUse", "dp pave-check", 1)
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
		"model":              "claude-sonnet-4-5-20250929",
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

// assertCommandCount verifies how many times one COMMAND appears under an
// event — which is what "idempotent" and "not clobbered" actually mean here.
//
// assertHookCount below counts hook ENTRY GROUPS, and the idempotence tests
// used to assert on that. It held only while dp installed exactly one hook per
// event, and it stopped holding the moment a genuinely new one was added:
// e6ddd92 gave `init` a `dp signpost` PostToolUse hook and a
// `dp signpost-prefetch` PreToolUse hook, and four tests went red reporting
// "count = 2, want 1" — a correct count of the wrong thing. Nothing was
// duplicated; there were simply two distinct hooks where the fixture expected
// one.
//
// Counting a command makes the assertion say what the test name says, and
// makes it survive the next hook.
func assertCommandCount(t *testing.T, hooks map[string]json.RawMessage, event, command string, want int) {
	t.Helper()
	raw, ok := hooks[event]
	if !ok {
		if want == 0 {
			return
		}
		t.Errorf("hook event %q not found (want %d occurrences of %q)", event, want, command)
		return
	}
	var entries []hookEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("parse %s entries: %v", event, err)
	}
	got := 0
	for _, e := range entries {
		for _, h := range e.Hooks {
			if h.Command == command {
				got++
			}
		}
	}
	if got != want {
		t.Errorf("%s: command %q appears %d time(s), want %d", event, command, got, want)
	}
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
