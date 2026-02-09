package source

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeCodeName(t *testing.T) {
	c := &claudeCode{}
	if got := c.Name(); got != "claude-code" {
		t.Errorf("Name() = %q, want %q", got, "claude-code")
	}
}

func TestClaudeCodeDescription(t *testing.T) {
	c := &claudeCode{}
	got := c.Description()
	if got == "" {
		t.Error("Description() should not be empty")
	}
	if got != "Claude Code PostToolUse/Failure hooks" {
		t.Errorf("Description() = %q, want %q", got, "Claude Code PostToolUse/Failure hooks")
	}
}

func TestClaudeCodeExtract(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
		check   func(t *testing.T, f *Fields)
	}{
		{
			name:  "full Claude Code hook payload",
			input: `{"session_id":"abc123","hook_event_name":"PostToolUseFailure","tool_name":"Bash","tool_input":{"command":"nonexistent-cmd"},"tool_use_id":"toolu_01xyz","error":"Command exited with non-zero status code 1","cwd":"/home/user/project","transcript_path":"/tmp/transcript","permission_mode":"default"}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "Bash" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "Bash")
				}
				if f.InstanceID != "abc123" {
					t.Errorf("InstanceID = %q, want %q", f.InstanceID, "abc123")
				}
				if f.CWD != "/home/user/project" {
					t.Errorf("CWD = %q, want %q", f.CWD, "/home/user/project")
				}
				if f.Error != "Command exited with non-zero status code 1" {
					t.Errorf("Error = %q, want %q", f.Error, "Command exited with non-zero status code 1")
				}

				// tool_input should be preserved as raw JSON.
				var ti map[string]string
				if err := json.Unmarshal(f.ToolInput, &ti); err != nil {
					t.Fatalf("unmarshaling ToolInput: %v", err)
				}
				if ti["command"] != "nonexistent-cmd" {
					t.Errorf("ToolInput.command = %q, want %q", ti["command"], "nonexistent-cmd")
				}

				// Extra should contain Claude-specific fields.
				if f.Extra == nil {
					t.Fatal("Extra should not be nil")
				}
				for _, key := range []string{"tool_use_id", "transcript_path", "hook_event_name", "permission_mode"} {
					if _, ok := f.Extra[key]; !ok {
						t.Errorf("Extra should contain %q", key)
					}
				}

				// Universal fields should NOT be in Extra.
				for _, key := range []string{"tool_name", "session_id", "cwd", "error", "tool_input"} {
					if _, ok := f.Extra[key]; ok {
						t.Errorf("Extra should not contain universal field %q", key)
					}
				}
			},
		},
		{
			name:  "minimal payload with only tool_name",
			input: `{"tool_name":"Read"}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "Read" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "Read")
				}
				if f.InstanceID != "" {
					t.Errorf("InstanceID = %q, want empty", f.InstanceID)
				}
				if f.CWD != "" {
					t.Errorf("CWD = %q, want empty", f.CWD)
				}
				if f.Error != "" {
					t.Errorf("Error = %q, want empty", f.Error)
				}
				if f.ToolInput != nil {
					t.Errorf("ToolInput = %s, want nil", f.ToolInput)
				}
				if f.Extra != nil {
					t.Errorf("Extra = %v, want nil", f.Extra)
				}
			},
		},
		{
			name:    "missing tool_name",
			input:   `{"session_id":"abc","error":"something"}`,
			wantErr: "missing required field: tool_name",
		},
		{
			name:    "empty tool_name",
			input:   `{"tool_name":""}`,
			wantErr: "missing required field: tool_name",
		},
		{
			name:    "invalid JSON",
			input:   `not json`,
			wantErr: "parsing JSON",
		},
		{
			name:    "tool_name is number",
			input:   `{"tool_name":123}`,
			wantErr: "parsing tool_name",
		},
		{
			name:  "unknown fields go to Extra",
			input: `{"tool_name":"Bash","custom_field":"value","another":42}`,
			check: func(t *testing.T, f *Fields) {
				if f.Extra == nil {
					t.Fatal("Extra should not be nil")
				}
				if _, ok := f.Extra["custom_field"]; !ok {
					t.Error("Extra should contain custom_field")
				}
				if _, ok := f.Extra["another"]; !ok {
					t.Error("Extra should contain another")
				}
			},
		},
		{
			name:  "tool_input preserved as raw JSON",
			input: `{"tool_name":"Write","tool_input":{"file_path":"/tmp/test","content":"hello"}}`,
			check: func(t *testing.T, f *Fields) {
				var ti map[string]string
				if err := json.Unmarshal(f.ToolInput, &ti); err != nil {
					t.Fatalf("unmarshaling ToolInput: %v", err)
				}
				if ti["file_path"] != "/tmp/test" {
					t.Errorf("ToolInput.file_path = %q, want %q", ti["file_path"], "/tmp/test")
				}
				if ti["content"] != "hello" {
					t.Errorf("ToolInput.content = %q, want %q", ti["content"], "hello")
				}
			},
		},
		{
			name:    "session_id is number",
			input:   `{"tool_name":"Bash","session_id":123}`,
			wantErr: "parsing session_id",
		},
		{
			name:    "cwd is array",
			input:   `{"tool_name":"Bash","cwd":["a"]}`,
			wantErr: "parsing cwd",
		},
		{
			name:    "error is object",
			input:   `{"tool_name":"Bash","error":{"msg":"fail"}}`,
			wantErr: "parsing error",
		},
	}

	c := &claudeCode{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := c.Extract([]byte(tt.input))

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			tt.check(t, f)
		})
	}
}

func TestClaudeCodeRegistered(t *testing.T) {
	// The init() function should have registered the plugin.
	s := Get("claude-code")
	if s == nil {
		t.Fatal("claude-code source not found in registry")
	}
	if s.Name() != "claude-code" {
		t.Errorf("Name() = %q, want %q", s.Name(), "claude-code")
	}
}

func TestClaudeCodeInstall(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, ".claude", "settings.json")

	c := &claudeCode{}
	if err := c.Install(InstallOpts{SettingsPath: settingsPath}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	// Verify the file was written.
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

	ptufRaw, ok := hooks["PostToolUseFailure"]
	if !ok {
		t.Fatal("hooks should contain PostToolUseFailure")
	}

	var entries []claudeHookEntry
	if err := json.Unmarshal(ptufRaw, &entries); err != nil {
		t.Fatalf("parsing PostToolUseFailure: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 hook entry, got %d", len(entries))
	}
	if entries[0].Hooks[0].Command != dpHookCommand {
		t.Errorf("command = %q, want %q", entries[0].Hooks[0].Command, dpHookCommand)
	}
}

func TestClaudeCodeInstallIdempotent(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, ".claude", "settings.json")

	c := &claudeCode{}

	// Install twice.
	if err := c.Install(InstallOpts{SettingsPath: settingsPath}); err != nil {
		t.Fatalf("first Install() error: %v", err)
	}
	if err := c.Install(InstallOpts{SettingsPath: settingsPath}); err != nil {
		t.Fatalf("second Install() error: %v", err)
	}

	// Should still have exactly one hook entry.
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings: %v", err)
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(settings["hooks"], &hooks); err != nil {
		t.Fatalf("parsing hooks: %v", err)
	}

	var entries []claudeHookEntry
	if err := json.Unmarshal(hooks["PostToolUseFailure"], &entries); err != nil {
		t.Fatalf("parsing PostToolUseFailure: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 hook entry after double install, got %d", len(entries))
	}
}

func TestClaudeCodeInstallPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Write existing settings with other hooks.
	existing := `{
  "hooks": {
    "PostToolUseFailure": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "other-tool record",
            "timeout": 3000
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "echo pre-bash",
            "timeout": 1000
          }
        ]
      }
    ]
  },
  "other_setting": "preserved"
}`
	if err := os.WriteFile(settingsPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &claudeCode{}
	if err := c.Install(InstallOpts{SettingsPath: settingsPath}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("reading settings: %v", err)
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parsing settings: %v", err)
	}

	// other_setting should be preserved.
	if _, ok := settings["other_setting"]; !ok {
		t.Error("other_setting should be preserved")
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(settings["hooks"], &hooks); err != nil {
		t.Fatalf("parsing hooks: %v", err)
	}

	// PreToolUse hook should be preserved.
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Error("PreToolUse hook should be preserved")
	}

	// PostToolUseFailure should now have 2 entries.
	var entries []claudeHookEntry
	if err := json.Unmarshal(hooks["PostToolUseFailure"], &entries); err != nil {
		t.Fatalf("parsing PostToolUseFailure: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 hook entries, got %d", len(entries))
	}

	// Original hook should be first.
	if entries[0].Hooks[0].Command != "other-tool record" {
		t.Errorf("first entry command = %q, want %q", entries[0].Hooks[0].Command, "other-tool record")
	}
	// dp hook should be second.
	if entries[1].Hooks[0].Command != dpHookCommand {
		t.Errorf("second entry command = %q, want %q", entries[1].Hooks[0].Command, dpHookCommand)
	}
}

func TestClaudeCodeIsInstalledNoFile(t *testing.T) {
	dir := t.TempDir()
	c := &claudeCode{}

	installed, err := c.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if installed {
		t.Error("IsInstalled() should be false when settings file does not exist")
	}
}

func TestClaudeCodeIsInstalledNoHooks(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(claudeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(`{"other_setting": "value"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &claudeCode{}
	installed, err := c.IsInstalled(claudeDir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if installed {
		t.Error("IsInstalled() should be false when no hooks are configured")
	}
}

func TestClaudeCodeIsInstalledWithDPRecord(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	c := &claudeCode{}
	// Install hooks first.
	if err := c.Install(InstallOpts{SettingsPath: settingsPath}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	installed, err := c.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if !installed {
		t.Error("IsInstalled() should be true after Install()")
	}
}

func TestClaudeCodeIsInstalledWithDPIngest(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	c := &claudeCode{}
	// Install with track-all to get dp ingest hooks.
	if err := c.Install(InstallOpts{SettingsPath: settingsPath, TrackAll: true}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	installed, err := c.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if !installed {
		t.Error("IsInstalled() should be true with dp ingest hooks")
	}
}

func TestClaudeCodeIsInstalledOtherHooksOnly(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "settings.json")

	// Write settings with non-dp hooks only.
	settings := `{
  "hooks": {
    "PostToolUseFailure": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "other-tool record",
            "timeout": 3000
          }
        ]
      }
    ]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &claudeCode{}
	installed, err := c.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if installed {
		t.Error("IsInstalled() should be false when only non-dp hooks exist")
	}
}

func TestClaudeCodeImplementsInstaller(t *testing.T) {
	var _ Installer = &claudeCode{}
}
