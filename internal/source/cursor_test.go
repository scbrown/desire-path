package source

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCursorName(t *testing.T) {
	c := &cursor{}
	if got := c.Name(); got != "cursor" {
		t.Errorf("Name() = %q, want %q", got, "cursor")
	}
}

func TestCursorDescription(t *testing.T) {
	c := &cursor{}
	got := c.Description()
	if got == "" {
		t.Error("Description() should not be empty")
	}
	if got != "Cursor IDE postToolUse/postToolUseFailure hooks" {
		t.Errorf("Description() = %q, want %q", got, "Cursor IDE postToolUse/postToolUseFailure hooks")
	}
}

func TestCursorExtract(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
		check   func(t *testing.T, f *Fields)
	}{
		{
			name:  "full postToolUse payload",
			input: `{"hook_event_name":"postToolUse","tool_name":"edit_file","tool_input":{"file_path":"/home/user/project/main.go","old_string":"func old()","new_string":"func new()"},"tool_output":"File edited successfully","tool_use_id":"toolu_abc123","conversation_id":"conv-xyz","generation_id":"gen-456","model":"cursor-fast","cursor_version":"2.2.44","cwd":"/home/user/project","duration":150,"transcript_path":"/tmp/cursor-transcript-abc.json"}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "edit_file" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "edit_file")
				}
				if f.InstanceID != "conv-xyz" {
					t.Errorf("InstanceID = %q, want %q", f.InstanceID, "conv-xyz")
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
				if ti["file_path"] != "/home/user/project/main.go" {
					t.Errorf("ToolInput.file_path = %q, want %q", ti["file_path"], "/home/user/project/main.go")
				}

				// Extra should contain Cursor-specific fields.
				if f.Extra == nil {
					t.Fatal("Extra should not be nil")
				}
				for _, key := range []string{"hook_event_name", "tool_output", "tool_use_id", "generation_id", "model", "cursor_version", "duration", "transcript_path"} {
					if _, ok := f.Extra[key]; !ok {
						t.Errorf("Extra should contain %q", key)
					}
				}

				// Universal fields should NOT be in Extra.
				for _, key := range []string{"tool_name", "conversation_id", "cwd", "error_message", "tool_input"} {
					if _, ok := f.Extra[key]; ok {
						t.Errorf("Extra should not contain universal field %q", key)
					}
				}
			},
		},
		{
			name:  "postToolUseFailure payload",
			input: `{"hook_event_name":"postToolUseFailure","tool_name":"edit_file","tool_input":{"file_path":"/etc/passwd"},"error_message":"Permission denied","failure_type":"denied","conversation_id":"conv-xyz","cwd":"/home/user/project"}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "edit_file" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "edit_file")
				}
				if f.Error != "Permission denied" {
					t.Errorf("Error = %q, want %q", f.Error, "Permission denied")
				}
				if f.InstanceID != "conv-xyz" {
					t.Errorf("InstanceID = %q, want %q", f.InstanceID, "conv-xyz")
				}

				// failure_type should be in Extra.
				if f.Extra == nil {
					t.Fatal("Extra should not be nil")
				}
				if _, ok := f.Extra["failure_type"]; !ok {
					t.Error("Extra should contain failure_type")
				}
			},
		},
		{
			name:  "minimal payload with only tool_name",
			input: `{"tool_name":"read_file"}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "read_file" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "read_file")
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
			input:   `{"conversation_id":"conv-1","error_message":"something"}`,
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
			name:    "conversation_id is number",
			input:   `{"tool_name":"edit_file","conversation_id":123}`,
			wantErr: "parsing conversation_id",
		},
		{
			name:    "cwd is array",
			input:   `{"tool_name":"edit_file","cwd":["a"]}`,
			wantErr: "parsing cwd",
		},
		{
			name:    "error_message is object",
			input:   `{"tool_name":"edit_file","error_message":{"msg":"fail"}}`,
			wantErr: "parsing error_message",
		},
		{
			name:  "unknown fields go to Extra",
			input: `{"tool_name":"edit_file","custom_field":"value","another":42}`,
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
			input: `{"tool_name":"create_file","tool_input":{"file_path":"/tmp/test","content":"hello"}}`,
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
			name:  "MCP tool name with namespace",
			input: `{"tool_name":"mcp_postgres_query","tool_input":{"query":"SELECT 1"},"cwd":"/tmp","conversation_id":"conv-1"}`,
			check: func(t *testing.T, f *Fields) {
				if f.ToolName != "mcp_postgres_query" {
					t.Errorf("ToolName = %q, want %q", f.ToolName, "mcp_postgres_query")
				}
			},
		},
	}

	c := &cursor{}
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

func TestCursorRegistered(t *testing.T) {
	s := Get("cursor")
	if s == nil {
		t.Fatal("cursor source not found in registry")
	}
	if s.Name() != "cursor" {
		t.Errorf("Name() = %q, want %q", s.Name(), "cursor")
	}
}

func TestCursorInstall(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, ".cursor", "hooks.json")

	c := &cursor{}
	if err := c.Install(InstallOpts{SettingsPath: hooksPath}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	// Verify the file was written.
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("reading hooks.json: %v", err)
	}

	var config cursorHooksConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parsing hooks.json: %v", err)
	}

	// Both postToolUse and postToolUseFailure should have dp ingest hooks.
	for _, event := range []string{"postToolUse", "postToolUseFailure"} {
		entry, ok := config.Hooks[event]
		if !ok {
			t.Fatalf("hooks should contain %s", event)
		}
		if entry.Command != dpCursorHookCommand {
			t.Errorf("%s: command = %q, want %q", event, entry.Command, dpCursorHookCommand)
		}
		if entry.Event != event {
			t.Errorf("%s: event = %q, want %q", event, entry.Event, event)
		}
	}
}

func TestCursorInstallIdempotent(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, ".cursor", "hooks.json")

	c := &cursor{}

	// Install twice.
	if err := c.Install(InstallOpts{SettingsPath: hooksPath}); err != nil {
		t.Fatalf("first Install() error: %v", err)
	}
	if err := c.Install(InstallOpts{SettingsPath: hooksPath}); err != nil {
		t.Fatalf("second Install() error: %v", err)
	}

	// Should still have exactly two hook entries (one per event).
	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("reading hooks.json: %v", err)
	}

	var config cursorHooksConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parsing hooks.json: %v", err)
	}

	if len(config.Hooks) != 2 {
		t.Fatalf("expected 2 hook entries after double install, got %d", len(config.Hooks))
	}
}

func TestCursorInstallPreservesExisting(t *testing.T) {
	dir := t.TempDir()
	cursorDir := filepath.Join(dir, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o700); err != nil {
		t.Fatal(err)
	}
	hooksPath := filepath.Join(cursorDir, "hooks.json")

	// Write existing hooks.json with another hook.
	existing := `{
  "hooks": {
    "preToolUse": {
      "command": "other-tool check",
      "event": "preToolUse"
    }
  }
}`
	if err := os.WriteFile(hooksPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &cursor{}
	if err := c.Install(InstallOpts{SettingsPath: hooksPath}); err != nil {
		t.Fatalf("Install() error: %v", err)
	}

	data, err := os.ReadFile(hooksPath)
	if err != nil {
		t.Fatalf("reading hooks.json: %v", err)
	}

	var config cursorHooksConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("parsing hooks.json: %v", err)
	}

	// preToolUse hook should be preserved.
	if _, ok := config.Hooks["preToolUse"]; !ok {
		t.Error("preToolUse hook should be preserved")
	}

	// dp hooks should be added.
	for _, event := range []string{"postToolUse", "postToolUseFailure"} {
		entry, ok := config.Hooks[event]
		if !ok {
			t.Fatalf("hooks should contain %s", event)
		}
		if entry.Command != dpCursorHookCommand {
			t.Errorf("%s: command = %q, want %q", event, entry.Command, dpCursorHookCommand)
		}
	}
}

func TestCursorIsInstalledNoFile(t *testing.T) {
	dir := t.TempDir()
	c := &cursor{}

	installed, err := c.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if installed {
		t.Error("IsInstalled() should be false when hooks.json does not exist")
	}
}

func TestCursorIsInstalledNoHooks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hooks.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &cursor{}
	installed, err := c.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if installed {
		t.Error("IsInstalled() should be false when no hooks are configured")
	}
}

func TestCursorIsInstalledWithDPHook(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, "hooks.json")

	c := &cursor{}
	// Install hooks first.
	if err := c.Install(InstallOpts{SettingsPath: hooksPath}); err != nil {
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

func TestCursorIsInstalledOtherHooksOnly(t *testing.T) {
	dir := t.TempDir()
	hooksPath := filepath.Join(dir, "hooks.json")

	config := `{
  "hooks": {
    "postToolUse": {
      "command": "other-tool record",
      "event": "postToolUse"
    }
  }
}`
	if err := os.WriteFile(hooksPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	c := &cursor{}
	installed, err := c.IsInstalled(dir)
	if err != nil {
		t.Fatalf("IsInstalled() error: %v", err)
	}
	if installed {
		t.Error("IsInstalled() should be false when only non-dp hooks exist")
	}
}

func TestCursorImplementsInstaller(t *testing.T) {
	var _ Installer = &cursor{}
}
