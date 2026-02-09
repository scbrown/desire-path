package source

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKiroName(t *testing.T) {
	k := &kiro{}
	if got := k.Name(); got != "kiro" {
		t.Errorf("Name() = %q, want %q", got, "kiro")
	}
}

func TestKiroDescription(t *testing.T) {
	k := &kiro{}
	got := k.Description()
	if got == "" {
		t.Error("Description() should not be empty")
	}
	if got != "Kiro CLI postToolUse hooks" {
		t.Errorf("Description() = %q, want %q", got, "Kiro CLI postToolUse hooks")
	}
}

func TestKiroExtract(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
		check   func(t *testing.T, f *Fields)
	}{
		{
			name:  "full Kiro postToolUse payload",
			input: `{"hook_event_name":"postToolUse","cwd":"/home/user/project","tool_name":"write","tool_input":{"file_path":"/tmp/test.go","content":"package main"},"tool_response":{"success":true,"result":["File written successfully"]}}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "write" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "write")
				}
				if f.CWD != "/home/user/project" {
					t.Errorf("CWD = %q, want %q", f.CWD, "/home/user/project")
				}
				if f.Error != "" {
					t.Errorf("Error = %q, want empty for successful call", f.Error)
				}

				// tool_input should be preserved as raw JSON.
				var ti map[string]string
				if err := json.Unmarshal(f.ToolInput, &ti); err != nil {
					t.Fatalf("unmarshaling ToolInput: %v", err)
				}
				if ti["file_path"] != "/tmp/test.go" {
					t.Errorf("ToolInput.file_path = %q, want %q", ti["file_path"], "/tmp/test.go")
				}

				// Extra should contain Kiro-specific fields.
				if f.Extra == nil {
					t.Fatal("Extra should not be nil")
				}
				for _, key := range []string{"hook_event_name", "tool_response"} {
					if _, ok := f.Extra[key]; !ok {
						t.Errorf("Extra should contain %q", key)
					}
				}

				// Universal fields should NOT be in Extra.
				for _, key := range []string{"tool_name", "tool_input", "cwd"} {
					if _, ok := f.Extra[key]; ok {
						t.Errorf("Extra should not contain universal field %q", key)
					}
				}
			},
		},
		{
			name:  "failed tool call sets Error",
			input: `{"hook_event_name":"postToolUse","cwd":"/home/user","tool_name":"shell","tool_input":{"command":"bad-cmd"},"tool_response":{"success":false,"result":["command not found"]}}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "shell" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "shell")
				}
				if f.Error != "tool call failed" {
					t.Errorf("Error = %q, want %q", f.Error, "tool call failed")
				}
			},
		},
		{
			name:  "MCP tool name with namespace",
			input: `{"hook_event_name":"postToolUse","tool_name":"@postgres/query","tool_input":{"query":"SELECT 1"},"cwd":"/tmp","tool_response":{"success":true,"result":["1"]}}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "@postgres/query" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "@postgres/query")
				}
			},
		},
		{
			name:  "minimal payload with only tool_name",
			input: `{"tool_name":"read"}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "read" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "read")
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
			input:   `{"hook_event_name":"postToolUse","cwd":"/tmp"}`,
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
			input: `{"tool_name":"glob","custom_field":"value","another":42}`,
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
			input: `{"tool_name":"write","tool_input":{"file_path":"/tmp/test","content":"hello"}}`,
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
			name:    "cwd is array",
			input:   `{"tool_name":"read","cwd":["a"]}`,
			wantErr: "parsing cwd",
		},
		{
			name:  "no tool_response does not set error",
			input: `{"tool_name":"read","cwd":"/tmp"}`,
			check: func(t *testing.T, f *Fields) {
				if f.Error != "" {
					t.Errorf("Error = %q, want empty when no tool_response", f.Error)
				}
			},
		},
		{
			name:  "malformed tool_response does not set error",
			input: `{"tool_name":"read","tool_response":"not-an-object"}`,
			check: func(t *testing.T, f *Fields) {
				if f.Error != "" {
					t.Errorf("Error = %q, want empty for malformed tool_response", f.Error)
				}
			},
		},
	}

	k := &kiro{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := k.Extract([]byte(tt.input))

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

func TestKiroRegistered(t *testing.T) {
	s := Get("kiro")
	if s == nil {
		t.Fatal("kiro source not found in registry")
	}
	if s.Name() != "kiro" {
		t.Errorf("Name() = %q, want %q", s.Name(), "kiro")
	}
}

func TestKiroInstall(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agents")

	k := &kiro{}
	if err := k.Install(InstallOpts{SettingsPath: agentDir}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	// Verify the file was written.
	agentPath := filepath.Join(agentDir, "dp-hooks.json")
	data, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("reading agent config: %v", err)
	}

	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parsing config: %v", err)
	}

	hooksRaw, ok := config["hooks"]
	if !ok {
		t.Fatal("config should contain hooks")
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooks); err != nil {
		t.Fatalf("parsing hooks: %v", err)
	}

	ptuRaw, ok := hooks["postToolUse"]
	if !ok {
		t.Fatal("hooks should contain postToolUse")
	}

	var entries []kiroHookEntry
	if err := json.Unmarshal(ptuRaw, &entries); err != nil {
		t.Fatalf("parsing postToolUse: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 hook entry, got %d", len(entries))
	}
	if entries[0].Command != dpKiroRecordCommand {
		t.Errorf("command = %q, want %q", entries[0].Command, dpKiroRecordCommand)
	}
	if entries[0].Matcher != "*" {
		t.Errorf("matcher = %q, want %q", entries[0].Matcher, "*")
	}
	if entries[0].TimeoutMs != 5000 {
		t.Errorf("timeout_ms = %d, want %d", entries[0].TimeoutMs, 5000)
	}
}

func TestKiroInstallTrackAll(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agents")

	k := &kiro{}
	if err := k.Install(InstallOpts{SettingsPath: agentDir, TrackAll: true}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	agentPath := filepath.Join(agentDir, "dp-hooks.json")
	data, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("reading agent config: %v", err)
	}

	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parsing config: %v", err)
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(config["hooks"], &hooks); err != nil {
		t.Fatalf("parsing hooks: %v", err)
	}

	var entries []kiroHookEntry
	if err := json.Unmarshal(hooks["postToolUse"], &entries); err != nil {
		t.Fatalf("parsing postToolUse: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 hook entry, got %d", len(entries))
	}
	if entries[0].Command != dpKiroIngestCommand {
		t.Errorf("command = %q, want %q", entries[0].Command, dpKiroIngestCommand)
	}
}

func TestKiroInstallIdempotent(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agents")

	k := &kiro{}

	// Install twice.
	if err := k.Install(InstallOpts{SettingsPath: agentDir}); err != nil {
		t.Fatalf("first Install() error: %v", err)
	}
	if err := k.Install(InstallOpts{SettingsPath: agentDir}); err != nil {
		t.Fatalf("second Install() error: %v", err)
	}

	// Should still have exactly one hook entry.
	agentPath := filepath.Join(agentDir, "dp-hooks.json")
	data, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("reading agent config: %v", err)
	}

	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parsing config: %v", err)
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(config["hooks"], &hooks); err != nil {
		t.Fatalf("parsing hooks: %v", err)
	}

	var entries []kiroHookEntry
	if err := json.Unmarshal(hooks["postToolUse"], &entries); err != nil {
		t.Fatalf("parsing postToolUse: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 hook entry after double install, got %d", len(entries))
	}
}

func TestKiroInstallPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agents")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	agentPath := filepath.Join(agentDir, "dp-hooks.json")

	// Write existing config with other hooks.
	existing := `{
  "hooks": {
    "postToolUse": [
      {
        "matcher": "@git",
        "command": "other-tool log",
        "timeout_ms": 3000
      }
    ],
    "preToolUse": [
      {
        "matcher": "shell",
        "command": "echo pre-shell",
        "timeout_ms": 1000
      }
    ]
  },
  "other_setting": "preserved"
}`
	if err := os.WriteFile(agentPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	k := &kiro{}
	if err := k.Install(InstallOpts{SettingsPath: agentDir}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	data, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf("reading agent config: %v", err)
	}

	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parsing config: %v", err)
	}

	// other_setting should be preserved.
	if _, ok := config["other_setting"]; !ok {
		t.Error("other_setting should be preserved")
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(config["hooks"], &hooks); err != nil {
		t.Fatalf("parsing hooks: %v", err)
	}

	// preToolUse hook should be preserved.
	if _, ok := hooks["preToolUse"]; !ok {
		t.Error("preToolUse hook should be preserved")
	}

	// postToolUse should now have 2 entries.
	var entries []kiroHookEntry
	if err := json.Unmarshal(hooks["postToolUse"], &entries); err != nil {
		t.Fatalf("parsing postToolUse: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 hook entries, got %d", len(entries))
	}

	// Original hook should be first.
	if entries[0].Command != "other-tool log" {
		t.Errorf("first entry command = %q, want %q", entries[0].Command, "other-tool log")
	}
	// dp hook should be second.
	if entries[1].Command != dpKiroRecordCommand {
		t.Errorf("second entry command = %q, want %q", entries[1].Command, dpKiroRecordCommand)
	}
}

func TestKiroIsInstalledNoFile(t *testing.T) {
	dir := t.TempDir()
	k := &kiro{}

	installed, err := k.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if installed {
		t.Error("IsInstalled() should be false when agent config does not exist")
	}
}

func TestKiroIsInstalledNoHooks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dp-hooks.json"), []byte(`{"other_setting": "value"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	k := &kiro{}
	installed, err := k.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if installed {
		t.Error("IsInstalled() should be false when no hooks are configured")
	}
}

func TestKiroIsInstalledWithDPRecord(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agents")

	k := &kiro{}
	if err := k.Install(InstallOpts{SettingsPath: agentDir}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	installed, err := k.IsInstalled(agentDir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if !installed {
		t.Error("IsInstalled() should be true after Install()")
	}
}

func TestKiroIsInstalledWithDPIngest(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agents")

	k := &kiro{}
	if err := k.Install(InstallOpts{SettingsPath: agentDir, TrackAll: true}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	installed, err := k.IsInstalled(agentDir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if !installed {
		t.Error("IsInstalled() should be true with dp ingest hooks")
	}
}

func TestKiroIsInstalledOtherHooksOnly(t *testing.T) {
	dir := t.TempDir()
	agentPath := filepath.Join(dir, "dp-hooks.json")

	config := `{
  "hooks": {
    "postToolUse": [
      {
        "matcher": "*",
        "command": "other-tool record",
        "timeout_ms": 3000
      }
    ]
  }
}`
	if err := os.WriteFile(agentPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	k := &kiro{}
	installed, err := k.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if installed {
		t.Error("IsInstalled() should be false when only non-dp hooks exist")
	}
}

func TestKiroImplementsInstaller(t *testing.T) {
	var _ Installer = &kiro{}
}
