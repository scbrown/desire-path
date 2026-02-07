package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupClaudeCodeCreatesNewFile(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, ".claude", "settings.json")

	// Capture stdout.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := setupClaudeCodeAt(settingsPath); err != nil {
		t.Fatalf("setupClaudeCodeAt: %v", err)
	}

	w.Close()
	os.Stdout = old
	var buf [4096]byte
	n, _ := r.Read(buf[:])
	output := string(buf[:n])

	// Verify output.
	if !strings.Contains(output, "Claude Code integration configured!") {
		t.Error("missing confirmation message")
	}
	if !strings.Contains(output, "PostToolUseFailure") {
		t.Error("missing hook event in output")
	}
	if !strings.Contains(output, dpHookCommand) {
		t.Error("missing hook command in output")
	}

	// Verify file was created.
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings: %v", err)
	}

	// Verify hooks.
	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(settings["hooks"], &hooks); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}

	var entries []claudeHookEntry
	if err := json.Unmarshal(hooks["PostToolUseFailure"], &entries); err != nil {
		t.Fatalf("parse PostToolUseFailure: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 hook entry, got %d", len(entries))
	}
	if entries[0].Matcher != ".*" {
		t.Errorf("matcher = %q, want %q", entries[0].Matcher, ".*")
	}
	if len(entries[0].Hooks) != 1 {
		t.Fatalf("expected 1 inner hook, got %d", len(entries[0].Hooks))
	}
	if entries[0].Hooks[0].Command != dpHookCommand {
		t.Errorf("command = %q, want %q", entries[0].Hooks[0].Command, dpHookCommand)
	}
	if entries[0].Hooks[0].Timeout != 5000 {
		t.Errorf("timeout = %d, want 5000", entries[0].Hooks[0].Timeout)
	}
}

func TestSetupClaudeCodeMergesWithExistingHooks(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Write existing settings with a Stop hook.
	existing := map[string]any{
		"permissions": map[string]any{
			"allow": []string{"Bash(curl:*)"},
		},
		"hooks": map[string]any{
			"Stop": []map[string]any{
				{
					"matcher": ".*",
					"hooks": []map[string]any{
						{"type": "command", "command": "echo done", "timeout": 3000},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Capture stdout.
	old := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	if err := setupClaudeCodeAt(settingsPath); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("setupClaudeCodeAt: %v", err)
	}
	w.Close()
	os.Stdout = old

	// Read back and verify both hook types exist.
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}

	// Verify permissions preserved.
	if _, ok := settings["permissions"]; !ok {
		t.Error("existing permissions field was lost")
	}

	// Verify hooks.
	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(settings["hooks"], &hooks); err != nil {
		t.Fatal(err)
	}

	// Stop hook should still exist.
	if _, ok := hooks["Stop"]; !ok {
		t.Error("existing Stop hook was clobbered")
	}

	// PostToolUseFailure should be added.
	var entries []claudeHookEntry
	if err := json.Unmarshal(hooks["PostToolUseFailure"], &entries); err != nil {
		t.Fatalf("parse PostToolUseFailure: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 PostToolUseFailure entry, got %d", len(entries))
	}
	if entries[0].Hooks[0].Command != dpHookCommand {
		t.Errorf("command = %q, want %q", entries[0].Hooks[0].Command, dpHookCommand)
	}
}

func TestSetupClaudeCodeAlreadyConfigured(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Write settings that already have the dp hook.
	existing := map[string]any{
		"hooks": map[string]any{
			"PostToolUseFailure": []map[string]any{
				{
					"matcher": ".*",
					"hooks": []map[string]any{
						{"type": "command", "command": dpHookCommand, "timeout": 5000},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Capture stderr (already configured message goes to stderr).
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	if err := setupClaudeCodeAt(settingsPath); err != nil {
		w.Close()
		os.Stderr = oldStderr
		t.Fatalf("setupClaudeCodeAt: %v", err)
	}
	w.Close()
	os.Stderr = oldStderr
	var buf [4096]byte
	n, _ := r.Read(buf[:])
	output := string(buf[:n])

	// Should say already configured, not add duplicate.
	if !strings.Contains(output, "already configured") {
		t.Error("expected 'already configured' message")
	}

	// Verify no duplicate hook was added.
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(settings["hooks"], &hooks); err != nil {
		t.Fatal(err)
	}
	var entries []claudeHookEntry
	if err := json.Unmarshal(hooks["PostToolUseFailure"], &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry (no duplicate), got %d", len(entries))
	}
}

func TestSetupClaudeCodePreservesExistingPostToolUseFailure(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Write settings with an existing PostToolUseFailure hook (not dp).
	existing := map[string]any{
		"hooks": map[string]any{
			"PostToolUseFailure": []map[string]any{
				{
					"matcher": ".*",
					"hooks": []map[string]any{
						{"type": "command", "command": "echo tool failed", "timeout": 3000},
					},
				},
			},
		},
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(settingsPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Capture stdout.
	old := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	if err := setupClaudeCodeAt(settingsPath); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("setupClaudeCodeAt: %v", err)
	}
	w.Close()
	os.Stdout = old

	// Read back and verify both entries exist.
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(settings["hooks"], &hooks); err != nil {
		t.Fatal(err)
	}
	var entries []claudeHookEntry
	if err := json.Unmarshal(hooks["PostToolUseFailure"], &entries); err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 PostToolUseFailure entries (existing + dp), got %d", len(entries))
	}
	// First entry should be the existing one.
	if entries[0].Hooks[0].Command != "echo tool failed" {
		t.Errorf("first entry command = %q, want %q", entries[0].Hooks[0].Command, "echo tool failed")
	}
	// Second entry should be dp.
	if entries[1].Hooks[0].Command != dpHookCommand {
		t.Errorf("second entry command = %q, want %q", entries[1].Hooks[0].Command, dpHookCommand)
	}
}

func TestSetupClaudeCodeWithEmptyExistingFile(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Write an empty JSON object.
	if err := os.WriteFile(settingsPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Capture stdout.
	old := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	if err := setupClaudeCodeAt(settingsPath); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("setupClaudeCodeAt: %v", err)
	}
	w.Close()
	os.Stdout = old

	// Read back and verify.
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(settings["hooks"], &hooks); err != nil {
		t.Fatal(err)
	}
	if _, ok := hooks["PostToolUseFailure"]; !ok {
		t.Error("PostToolUseFailure hook not added to empty settings")
	}
}

func TestInitCmdRequiresFlag(t *testing.T) {
	// Reset flag state.
	oldFlag := initClaudeCode
	defer func() { initClaudeCode = oldFlag }()
	initClaudeCode = false

	err := initCmd.RunE(initCmd, nil)
	if err == nil {
		t.Fatal("expected error when no flag is specified")
	}
	if !strings.Contains(err.Error(), "--claude-code") {
		t.Errorf("error should mention --claude-code, got: %v", err)
	}
}

func TestHasDPHook(t *testing.T) {
	tests := []struct {
		name    string
		entries []claudeHookEntry
		want    bool
	}{
		{
			name:    "empty entries",
			entries: nil,
			want:    false,
		},
		{
			name: "different hook",
			entries: []claudeHookEntry{
				{
					Matcher: ".*",
					Hooks:   []claudeHookInner{{Type: "command", Command: "echo test"}},
				},
			},
			want: false,
		},
		{
			name: "dp hook present",
			entries: []claudeHookEntry{
				{
					Matcher: ".*",
					Hooks:   []claudeHookInner{{Type: "command", Command: dpHookCommand}},
				},
			},
			want: true,
		},
		{
			name: "dp hook among others",
			entries: []claudeHookEntry{
				{
					Matcher: ".*",
					Hooks:   []claudeHookInner{{Type: "command", Command: "echo first"}},
				},
				{
					Matcher: ".*",
					Hooks:   []claudeHookInner{{Type: "command", Command: dpHookCommand}},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasDPHook(tt.entries)
			if got != tt.want {
				t.Errorf("hasDPHook() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadClaudeSettingsMissing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent", "settings.json")
	settings, err := readClaudeSettings(path)
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if len(settings) != 0 {
		t.Errorf("expected empty settings, got %d keys", len(settings))
	}
}

func TestReadClaudeSettingsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := readClaudeSettings(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error should mention parse, got: %v", err)
	}
}
