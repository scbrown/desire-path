package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// knownClaudeFields lists JSON keys from Claude Code hook payloads that map
// to universal Fields. Everything else goes into Extra.
var knownClaudeFields = map[string]bool{
	"tool_name":  true,
	"session_id": true,
	"tool_input": true,
	"cwd":        true,
	"error":      true,
}

// claudeCode implements Source for Claude Code's PostToolUseFailure hook.
type claudeCode struct{}

func init() {
	Register(&claudeCode{})
}

// Name returns "claude-code".
func (c *claudeCode) Name() string { return "claude-code" }

// Extract parses Claude Code hook JSON and maps fields to the universal
// Fields struct. Claude Code provides: session_id, hook_event_name,
// tool_name, tool_input, tool_use_id, error, cwd, transcript_path,
// permission_mode. Unknown keys are placed into Extra alongside the
// Claude-specific keys listed above.
func (c *claudeCode) Extract(raw []byte) (*Fields, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("claude-code: parsing JSON: %w", err)
	}

	var f Fields

	// tool_name is required.
	tn, ok := m["tool_name"]
	if !ok {
		return nil, fmt.Errorf("claude-code: missing required field: tool_name")
	}
	if err := json.Unmarshal(tn, &f.ToolName); err != nil {
		return nil, fmt.Errorf("claude-code: parsing tool_name: %w", err)
	}
	if f.ToolName == "" {
		return nil, fmt.Errorf("claude-code: missing required field: tool_name")
	}

	// session_id → InstanceID
	if v, ok := m["session_id"]; ok {
		if err := json.Unmarshal(v, &f.InstanceID); err != nil {
			return nil, fmt.Errorf("claude-code: parsing session_id: %w", err)
		}
	}

	// tool_input → ToolInput (preserved as raw JSON)
	if v, ok := m["tool_input"]; ok {
		f.ToolInput = v
	}

	// cwd → CWD
	if v, ok := m["cwd"]; ok {
		if err := json.Unmarshal(v, &f.CWD); err != nil {
			return nil, fmt.Errorf("claude-code: parsing cwd: %w", err)
		}
	}

	// error → Error
	if v, ok := m["error"]; ok {
		if err := json.Unmarshal(v, &f.Error); err != nil {
			return nil, fmt.Errorf("claude-code: parsing error: %w", err)
		}
	}

	// Collect everything not in knownClaudeFields into Extra.
	// This includes tool_use_id, transcript_path, hook_event_name,
	// permission_mode, and any future Claude Code fields.
	extra := make(map[string]json.RawMessage)
	for k, v := range m {
		if !knownClaudeFields[k] {
			extra[k] = v
		}
	}
	if len(extra) > 0 {
		f.Extra = extra
	}

	return &f, nil
}

// Install configures the Claude Code PostToolUseFailure hook by merging
// dp's hook entry into the settings file at settingsPath. If settingsPath
// is empty, the default ~/.claude/settings.json is used.
func (c *claudeCode) Install(settingsPath string) error {
	if settingsPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("determine home directory: %w", err)
		}
		settingsPath = filepath.Join(home, ".claude", "settings.json")
	}
	return setupClaudeCodeAt(settingsPath)
}

// claudeSettings represents the relevant subset of a Claude Code settings file.
type claudeSettings map[string]json.RawMessage

// claudeHookEntry represents a single hook entry in the hooks config.
type claudeHookEntry struct {
	Matcher string            `json:"matcher"`
	Hooks   []claudeHookInner `json:"hooks"`
}

// claudeHookInner represents the inner hook command definition.
type claudeHookInner struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// dpHookCommand is the command dp installs for PostToolUseFailure.
const dpHookCommand = "dp record --source claude-code"

// setupClaudeCodeAt performs the Claude Code setup using the given settings path.
func setupClaudeCodeAt(settingsPath string) error {
	settings, err := readClaudeSettings(settingsPath)
	if err != nil {
		return err
	}

	dpHook := claudeHookEntry{
		Matcher: ".*",
		Hooks: []claudeHookInner{
			{
				Type:    "command",
				Command: dpHookCommand,
				Timeout: 5000,
			},
		},
	}

	hooks, err := mergeHookEvent(settings, "PostToolUseFailure", dpHook)
	if err != nil {
		return err
	}

	hooksJSON, err := json.Marshal(hooks)
	if err != nil {
		return fmt.Errorf("marshal hooks: %w", err)
	}
	settings["hooks"] = hooksJSON

	return writeClaudeSettings(settingsPath, settings)
}

// readClaudeSettings reads and parses the settings file, returning an empty
// map if the file does not exist.
func readClaudeSettings(path string) (claudeSettings, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(claudeSettings), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var s claudeSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return s, nil
}

// mergeHookEvent merges dpHook into the named event's hook list without
// clobbering existing hooks.
func mergeHookEvent(settings claudeSettings, eventName string, dpHook claudeHookEntry) (map[string]json.RawMessage, error) {
	hooks := make(map[string]json.RawMessage)
	if raw, ok := settings["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return nil, fmt.Errorf("parse existing hooks: %w", err)
		}
	}

	var entries []claudeHookEntry
	if raw, ok := hooks[eventName]; ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, fmt.Errorf("parse %s hooks: %w", eventName, err)
		}
	}

	if hasDPHookCommand(entries, dpHook.Hooks[0].Command) {
		return hooks, nil
	}

	entries = append(entries, dpHook)
	entriesJSON, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", eventName, err)
	}
	hooks[eventName] = entriesJSON

	return hooks, nil
}

// hasDPHookCommand returns true if entries already contain a hook running
// the given command.
func hasDPHookCommand(entries []claudeHookEntry, command string) bool {
	for _, e := range entries {
		for _, h := range e.Hooks {
			if h.Command == command {
				return true
			}
		}
	}
	return false
}

// writeClaudeSettings writes settings back to the file, creating the parent
// directory if needed.
func writeClaudeSettings(path string, settings claudeSettings) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
