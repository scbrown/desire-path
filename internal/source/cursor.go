package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// knownCursorFields lists JSON keys from Cursor hook payloads that map
// to universal Fields. Everything else goes into Extra.
var knownCursorFields = map[string]bool{
	"tool_name":       true,
	"tool_input":      true,
	"cwd":             true,
	"conversation_id": true,
	"error_message":   true,
}

// cursor implements Source for Cursor IDE's postToolUse/postToolUseFailure hooks.
type cursor struct{}

func init() {
	Register(&cursor{})
}

// Name returns "cursor".
func (c *cursor) Name() string { return "cursor" }

// Description returns a short human-readable description of this source.
func (c *cursor) Description() string { return "Cursor IDE postToolUse/postToolUseFailure hooks" }

// Extract parses Cursor IDE hook JSON and maps fields to the universal
// Fields struct. Cursor provides: hook_event_name, tool_name, tool_input,
// tool_output, tool_use_id, conversation_id, generation_id, model,
// cursor_version, cwd, duration, transcript_path. For failure events,
// error_message and failure_type are provided instead of tool_output.
func (c *cursor) Extract(raw []byte) (*Fields, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("cursor: parsing JSON: %w", err)
	}

	var f Fields

	// tool_name is required.
	tn, ok := m["tool_name"]
	if !ok {
		return nil, fmt.Errorf("cursor: missing required field: tool_name")
	}
	if err := json.Unmarshal(tn, &f.ToolName); err != nil {
		return nil, fmt.Errorf("cursor: parsing tool_name: %w", err)
	}
	if f.ToolName == "" {
		return nil, fmt.Errorf("cursor: missing required field: tool_name")
	}

	// conversation_id → InstanceID
	if v, ok := m["conversation_id"]; ok {
		if err := json.Unmarshal(v, &f.InstanceID); err != nil {
			return nil, fmt.Errorf("cursor: parsing conversation_id: %w", err)
		}
	}

	// tool_input → ToolInput (preserved as raw JSON)
	if v, ok := m["tool_input"]; ok {
		f.ToolInput = v
	}

	// cwd → CWD
	if v, ok := m["cwd"]; ok {
		if err := json.Unmarshal(v, &f.CWD); err != nil {
			return nil, fmt.Errorf("cursor: parsing cwd: %w", err)
		}
	}

	// error_message → Error (from postToolUseFailure events)
	if v, ok := m["error_message"]; ok {
		if err := json.Unmarshal(v, &f.Error); err != nil {
			return nil, fmt.Errorf("cursor: parsing error_message: %w", err)
		}
	}

	// Collect everything not in knownCursorFields into Extra.
	extra := make(map[string]json.RawMessage)
	for k, v := range m {
		if !knownCursorFields[k] {
			extra[k] = v
		}
	}
	if len(extra) > 0 {
		f.Extra = extra
	}

	return &f, nil
}

// cursorHooksConfig represents the structure of Cursor's hooks.json file.
type cursorHooksConfig struct {
	Hooks map[string]cursorHookEntry `json:"hooks"`
}

// cursorHookEntry represents a single hook entry in Cursor's hooks config.
type cursorHookEntry struct {
	Command string `json:"command"`
	Event   string `json:"event"`
}

// dpCursorHookCommand is the canonical command installed for both hook events.
const dpCursorHookCommand = "dp ingest --source cursor"

// Install configures Cursor IDE hooks. It installs dp ingest hooks on
// both postToolUse and postToolUseFailure events. opts.TrackAll is accepted
// but ignored (all invocations are always tracked).
func (c *cursor) Install(opts InstallOpts) error {
	hooksPath := opts.SettingsPath
	if hooksPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("determine home directory: %w", err)
		}
		hooksPath = filepath.Join(home, ".cursor", "hooks.json")
	}
	return installCursorHooks(hooksPath)
}

// IsInstalled checks whether dp hooks are already configured in Cursor's
// hooks.json file. configDir should point to the directory containing
// hooks.json (e.g., ~/.cursor).
func (c *cursor) IsInstalled(configDir string) (bool, error) {
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false, fmt.Errorf("determine home directory: %w", err)
		}
		configDir = filepath.Join(home, ".cursor")
	}
	hooksPath := filepath.Join(configDir, "hooks.json")

	config, err := readCursorHooksConfig(hooksPath)
	if err != nil {
		return false, err
	}

	for _, entry := range config.Hooks {
		if entry.Command == dpCursorHookCommand {
			return true, nil
		}
	}

	return false, nil
}

// installCursorHooks writes or merges Cursor hooks.json with dp ingest hooks
// for postToolUse and postToolUseFailure events.
func installCursorHooks(hooksPath string) error {
	config, err := readCursorHooksConfig(hooksPath)
	if err != nil {
		return err
	}

	if config.Hooks == nil {
		config.Hooks = make(map[string]cursorHookEntry)
	}

	events := []string{"postToolUse", "postToolUseFailure"}
	for _, event := range events {
		if existing, ok := config.Hooks[event]; ok && existing.Command == dpCursorHookCommand {
			continue
		}
		config.Hooks[event] = cursorHookEntry{
			Command: dpCursorHookCommand,
			Event:   event,
		}
	}

	return writeCursorHooksConfig(hooksPath, config)
}

// readCursorHooksConfig reads and parses Cursor's hooks.json file,
// returning an empty config if the file does not exist.
func readCursorHooksConfig(path string) (cursorHooksConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cursorHooksConfig{Hooks: make(map[string]cursorHookEntry)}, nil
	}
	if err != nil {
		return cursorHooksConfig{}, fmt.Errorf("read %s: %w", path, err)
	}

	var config cursorHooksConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return cursorHooksConfig{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if config.Hooks == nil {
		config.Hooks = make(map[string]cursorHookEntry)
	}
	return config, nil
}

// writeCursorHooksConfig writes the hooks config to disk, creating the parent
// directory if needed.
func writeCursorHooksConfig(path string, config cursorHooksConfig) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
