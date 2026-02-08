package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// --- test doubles ---

// stubSource is a minimal Source implementation for registry tests.
type stubSource struct {
	name string
}

func (s *stubSource) Name() string                   { return s.name }
func (s *stubSource) Extract([]byte) (*Fields, error) { return &Fields{ToolName: "stub"}, nil }

// claudeCodeSource is a test double that mimics a Claude Code source plugin.
// It parses the hook payload format that Claude Code sends via
// PostToolUseFailure and maps fields to the universal Fields struct.
type claudeCodeSource struct{}

func (c *claudeCodeSource) Name() string { return "claude-code" }

func (c *claudeCodeSource) Extract(raw []byte) (*Fields, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parsing payload: %w", err)
	}

	f := &Fields{}

	// tool_name is required.
	tn, ok := payload["tool_name"]
	if !ok {
		return nil, fmt.Errorf("missing required field: tool_name")
	}
	if err := json.Unmarshal(tn, &f.ToolName); err != nil {
		return nil, fmt.Errorf("parsing tool_name: %w", err)
	}
	if f.ToolName == "" {
		return nil, fmt.Errorf("missing required field: tool_name")
	}

	// Optional universal fields.
	if v, ok := payload["session_id"]; ok {
		json.Unmarshal(v, &f.InstanceID)
	}
	if v, ok := payload["tool_input"]; ok {
		f.ToolInput = v
	}
	if v, ok := payload["cwd"]; ok {
		json.Unmarshal(v, &f.CWD)
	}
	if v, ok := payload["error"]; ok {
		json.Unmarshal(v, &f.Error)
	}

	// Everything else goes into Extra.
	universal := map[string]bool{
		"tool_name": true, "session_id": true, "tool_input": true,
		"cwd": true, "error": true,
	}
	for k, v := range payload {
		if universal[k] {
			continue
		}
		if f.Extra == nil {
			f.Extra = make(map[string]json.RawMessage)
		}
		f.Extra[k] = v
	}

	return f, nil
}

// claudeCodeInstaller is a test double that implements the Installer
// interface. It merges a PostToolUseFailure hook into a Claude Code
// settings file without overwriting existing hooks.
type claudeCodeInstaller struct {
	claudeCodeSource
}

func (c *claudeCodeInstaller) Install(opts InstallOpts) error {
	settingsPath := opts.SettingsPath
	const hookCommand = "dp record --source claude-code"

	// Read existing settings or start fresh.
	settings := make(map[string]json.RawMessage)
	data, err := os.ReadFile(settingsPath)
	if err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parse settings: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read settings: %w", err)
	}

	// Parse existing hooks.
	hooks := make(map[string]json.RawMessage)
	if raw, ok := settings["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return fmt.Errorf("parse hooks: %w", err)
		}
	}

	// Parse existing PostToolUseFailure entries.
	type hookInner struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	type hookEntry struct {
		Matcher string      `json:"matcher"`
		Hooks   []hookInner `json:"hooks"`
	}

	var entries []hookEntry
	if raw, ok := hooks["PostToolUseFailure"]; ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return fmt.Errorf("parse PostToolUseFailure: %w", err)
		}
	}

	// Check if already present (idempotency).
	for _, e := range entries {
		for _, h := range e.Hooks {
			if h.Command == hookCommand {
				return nil // already configured
			}
		}
	}

	// Append dp hook.
	entries = append(entries, hookEntry{
		Matcher: ".*",
		Hooks:   []hookInner{{Type: "command", Command: hookCommand, Timeout: 5000}},
	})

	entriesJSON, _ := json.Marshal(entries)
	hooks["PostToolUseFailure"] = entriesJSON
	hooksJSON, _ := json.Marshal(hooks)
	settings["hooks"] = hooksJSON

	// Write back.
	dir := filepath.Dir(settingsPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	return os.WriteFile(settingsPath, append(out, '\n'), 0o644)
}

// --- registry tests ---

func TestRegisterAndGet(t *testing.T) {
	name := "test-register-get"
	s := &stubSource{name: name}
	Register(s)

	got := Get(name)
	if got == nil {
		t.Fatalf("Get(%q) = nil, want registered source", name)
	}
	if got.Name() != name {
		t.Errorf("Name() = %q, want %q", got.Name(), name)
	}
}

func TestGetUnregistered(t *testing.T) {
	got := Get("nonexistent-source")
	if got != nil {
		t.Errorf("Get(nonexistent) = %v, want nil", got)
	}
}

func TestNames(t *testing.T) {
	// Register sources with names that sort predictably.
	Register(&stubSource{name: "zzz-alpha"})
	Register(&stubSource{name: "zzz-beta"})
	Register(&stubSource{name: "zzz-gamma"})

	names := Names()

	// Verify our names are present and sorted.
	found := map[string]bool{}
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("Names() not sorted: %q before %q", names[i-1], names[i])
		}
	}
	for _, n := range names {
		found[n] = true
	}
	for _, want := range []string{"zzz-alpha", "zzz-beta", "zzz-gamma"} {
		if !found[want] {
			t.Errorf("Names() missing %q", want)
		}
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	name := "test-dup-panic"
	Register(&stubSource{name: name})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on duplicate registration, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is %T, want string", r)
		}
		if got := msg; got == "" {
			t.Error("panic message should not be empty")
		}
	}()

	Register(&stubSource{name: name}) // should panic
}

// --- Claude Code Extract tests ---

func TestClaudeCodeExtractFullPayload(t *testing.T) {
	src := &claudeCodeSource{}

	payload := `{
		"tool_name": "Bash",
		"session_id": "sess-abc123",
		"tool_input": {"command": "ls -la"},
		"cwd": "/home/user/project",
		"error": "Command exited with non-zero status code 1",
		"hook_event_name": "PostToolUseFailure",
		"tool_use_id": "toolu_01xyz",
		"transcript_path": "/tmp/transcript.json",
		"permission_mode": "default"
	}`

	f, err := src.Extract([]byte(payload))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if f.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want %q", f.ToolName, "Bash")
	}
	if f.InstanceID != "sess-abc123" {
		t.Errorf("InstanceID = %q, want %q", f.InstanceID, "sess-abc123")
	}
	if f.CWD != "/home/user/project" {
		t.Errorf("CWD = %q, want %q", f.CWD, "/home/user/project")
	}
	if f.Error != "Command exited with non-zero status code 1" {
		t.Errorf("Error = %q, want %q", f.Error, "Command exited with non-zero status code 1")
	}

	// Verify tool_input is preserved as raw JSON.
	var ti map[string]string
	if err := json.Unmarshal(f.ToolInput, &ti); err != nil {
		t.Fatalf("unmarshal ToolInput: %v", err)
	}
	if ti["command"] != "ls -la" {
		t.Errorf("ToolInput.command = %q, want %q", ti["command"], "ls -la")
	}
}

func TestClaudeCodeExtractMinimalPayload(t *testing.T) {
	src := &claudeCodeSource{}

	payload := `{"tool_name": "Read"}`
	f, err := src.Extract([]byte(payload))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

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
}

func TestClaudeCodeExtractMissingToolName(t *testing.T) {
	src := &claudeCodeSource{}

	payload := `{"error": "something failed", "cwd": "/tmp"}`
	_, err := src.Extract([]byte(payload))
	if err == nil {
		t.Fatal("expected error for missing tool_name, got nil")
	}
	if got := err.Error(); got != "missing required field: tool_name" {
		t.Errorf("error = %q, want %q", got, "missing required field: tool_name")
	}
}

func TestClaudeCodeExtractEmptyToolName(t *testing.T) {
	src := &claudeCodeSource{}

	payload := `{"tool_name": ""}`
	_, err := src.Extract([]byte(payload))
	if err == nil {
		t.Fatal("expected error for empty tool_name, got nil")
	}
	if got := err.Error(); got != "missing required field: tool_name" {
		t.Errorf("error = %q, want %q", got, "missing required field: tool_name")
	}
}

func TestClaudeCodeExtractInvalidJSON(t *testing.T) {
	src := &claudeCodeSource{}

	_, err := src.Extract([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// --- Extra field tests ---

func TestSourceSpecificFieldsLandInExtra(t *testing.T) {
	src := &claudeCodeSource{}

	payload := `{
		"tool_name": "Bash",
		"cwd": "/tmp",
		"hook_event_name": "PostToolUseFailure",
		"tool_use_id": "toolu_01xyz",
		"transcript_path": "/tmp/transcript.json",
		"permission_mode": "default",
		"custom_field": "custom_value"
	}`

	f, err := src.Extract([]byte(payload))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Universal fields must NOT be in Extra.
	for _, key := range []string{"tool_name", "session_id", "tool_input", "cwd", "error"} {
		if _, ok := f.Extra[key]; ok {
			t.Errorf("universal field %q should not be in Extra", key)
		}
	}

	// Source-specific fields MUST be in Extra.
	wantExtra := []string{"hook_event_name", "tool_use_id", "transcript_path", "permission_mode", "custom_field"}
	for _, key := range wantExtra {
		if _, ok := f.Extra[key]; !ok {
			t.Errorf("source-specific field %q should be in Extra", key)
		}
	}

	// Verify Extra values are correct.
	var hookEvent string
	if err := json.Unmarshal(f.Extra["hook_event_name"], &hookEvent); err != nil {
		t.Fatalf("unmarshal hook_event_name: %v", err)
	}
	if hookEvent != "PostToolUseFailure" {
		t.Errorf("Extra[hook_event_name] = %q, want %q", hookEvent, "PostToolUseFailure")
	}

	var custom string
	if err := json.Unmarshal(f.Extra["custom_field"], &custom); err != nil {
		t.Fatalf("unmarshal custom_field: %v", err)
	}
	if custom != "custom_value" {
		t.Errorf("Extra[custom_field] = %q, want %q", custom, "custom_value")
	}
}

// --- Installer tests ---

func TestClaudeCodeInstallerCreatesHooks(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, ".claude", "settings.json")

	installer := &claudeCodeInstaller{}
	if err := installer.Install(InstallOpts{SettingsPath: settingsPath}); err != nil {
		t.Fatalf("Install: %v", err)
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

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(settings["hooks"], &hooks); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}

	type hookInner struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	type hookEntry struct {
		Matcher string      `json:"matcher"`
		Hooks   []hookInner `json:"hooks"`
	}

	var entries []hookEntry
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
	if entries[0].Hooks[0].Command != "dp record --source claude-code" {
		t.Errorf("command = %q, want %q", entries[0].Hooks[0].Command, "dp record --source claude-code")
	}
	if entries[0].Hooks[0].Timeout != 5000 {
		t.Errorf("timeout = %d, want 5000", entries[0].Hooks[0].Timeout)
	}
}

func TestClaudeCodeInstallerIdempotent(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, ".claude", "settings.json")

	installer := &claudeCodeInstaller{}

	// Install twice.
	if err := installer.Install(InstallOpts{SettingsPath: settingsPath}); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	if err := installer.Install(InstallOpts{SettingsPath: settingsPath}); err != nil {
		t.Fatalf("second Install: %v", err)
	}

	// Verify no duplicate hook was added.
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read settings: %v", err)
	}

	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("parse settings: %v", err)
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(settings["hooks"], &hooks); err != nil {
		t.Fatalf("parse hooks: %v", err)
	}

	type hookInner struct {
		Type    string `json:"type"`
		Command string `json:"command"`
		Timeout int    `json:"timeout"`
	}
	type hookEntry struct {
		Matcher string      `json:"matcher"`
		Hooks   []hookInner `json:"hooks"`
	}

	var entries []hookEntry
	if err := json.Unmarshal(hooks["PostToolUseFailure"], &entries); err != nil {
		t.Fatalf("parse PostToolUseFailure: %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("expected 1 entry after double install (idempotent), got %d", len(entries))
	}
}

func TestInstallerInterface(t *testing.T) {
	// Verify claudeCodeInstaller satisfies both Source and Installer.
	var s Source = &claudeCodeInstaller{}
	if s.Name() != "claude-code" {
		t.Errorf("Name() = %q, want %q", s.Name(), "claude-code")
	}

	var i Installer = &claudeCodeInstaller{}
	// Verify Install works with a fresh temp path.
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := i.Install(InstallOpts{SettingsPath: path}); err != nil {
		t.Fatalf("Install: %v", err)
	}
}
