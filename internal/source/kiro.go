package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// knownKiroFields lists JSON keys from Kiro CLI hook payloads that map
// to universal Fields. Everything else goes into Extra.
var knownKiroFields = map[string]bool{
	"tool_name":  true,
	"tool_input": true,
	"cwd":        true,
}

// kiro implements Source for Kiro CLI's postToolUse hook.
type kiro struct{}

func init() {
	Register(&kiro{})
}

// Name returns "kiro".
func (k *kiro) Name() string { return "kiro" }

// Description returns a short human-readable description of this source.
func (k *kiro) Description() string { return "Kiro CLI postToolUse hooks" }

// Extract parses Kiro CLI hook JSON and maps fields to the universal
// Fields struct. Kiro provides: hook_event_name, tool_name, tool_input,
// tool_response, cwd. There is no separate session_id field; there is no
// separate error field — errors are indicated by tool_response.success=false.
func (k *kiro) Extract(raw []byte) (*Fields, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("kiro: parsing JSON: %w", err)
	}

	var f Fields

	// tool_name is required.
	tn, ok := m["tool_name"]
	if !ok {
		return nil, fmt.Errorf("kiro: missing required field: tool_name")
	}
	if err := json.Unmarshal(tn, &f.ToolName); err != nil {
		return nil, fmt.Errorf("kiro: parsing tool_name: %w", err)
	}
	if f.ToolName == "" {
		return nil, fmt.Errorf("kiro: missing required field: tool_name")
	}

	// tool_input → ToolInput (preserved as raw JSON)
	if v, ok := m["tool_input"]; ok {
		f.ToolInput = v
	}

	// cwd → CWD
	if v, ok := m["cwd"]; ok {
		if err := json.Unmarshal(v, &f.CWD); err != nil {
			return nil, fmt.Errorf("kiro: parsing cwd: %w", err)
		}
	}

	// Kiro signals errors via tool_response.success=false rather than a
	// dedicated error field. Extract the error state if present.
	if v, ok := m["tool_response"]; ok {
		var resp struct {
			Success bool `json:"success"`
		}
		if err := json.Unmarshal(v, &resp); err == nil && !resp.Success {
			f.Error = "tool call failed"
		}
	}

	// Collect everything not in knownKiroFields into Extra.
	// This includes hook_event_name, tool_response, and any future fields.
	extra := make(map[string]json.RawMessage)
	for key, v := range m {
		if !knownKiroFields[key] {
			extra[key] = v
		}
	}
	if len(extra) > 0 {
		f.Extra = extra
	}

	return &f, nil
}

// kiroSettingsDir returns the default Kiro CLI settings directory.
func kiroSettingsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determine home directory: %w", err)
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, ".kiro"), nil
	}
	// Linux: XDG convention
	return filepath.Join(home, ".config", "kiro"), nil
}

// kiroAgentConfig represents the structure of a Kiro agent JSON file.
type kiroAgentConfig map[string]json.RawMessage

// kiroHookEntry represents a single hook entry in Kiro agent config.
type kiroHookEntry struct {
	Matcher   string `json:"matcher"`
	Command   string `json:"command"`
	TimeoutMs int    `json:"timeout_ms"`
}

// dpKiroRecordCommand is the command dp installs for postToolUse (failure detection).
const dpKiroRecordCommand = "dp record --source kiro"

// dpKiroIngestCommand is the command for recording all invocations.
const dpKiroIngestCommand = "dp ingest --source kiro"

// Install configures Kiro CLI hooks. By default it installs a postToolUse → dp record
// hook. When opts.TrackAll is true, it installs dp ingest on postToolUse to record
// all invocations.
func (k *kiro) Install(opts InstallOpts) error {
	agentDir := opts.SettingsPath
	if agentDir == "" {
		dir, err := kiroSettingsDir()
		if err != nil {
			return err
		}
		agentDir = filepath.Join(dir, "agents")
	}
	agentPath := filepath.Join(agentDir, "dp-hooks.json")

	command := dpKiroRecordCommand
	if opts.TrackAll {
		command = dpKiroIngestCommand
	}

	return installKiroHooks(agentPath, command)
}

// installKiroHooks writes or merges a Kiro agent config file containing
// the dp postToolUse hook.
func installKiroHooks(agentPath string, command string) error {
	config, err := readKiroAgentConfig(agentPath)
	if err != nil {
		return err
	}

	hooks := make(map[string]json.RawMessage)
	if raw, ok := config["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return fmt.Errorf("parse existing hooks: %w", err)
		}
	}

	var entries []kiroHookEntry
	if raw, ok := hooks["postToolUse"]; ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return fmt.Errorf("parse postToolUse hooks: %w", err)
		}
	}

	if hasKiroDPCommand(entries, command) {
		return nil
	}

	entries = append(entries, kiroHookEntry{
		Matcher:   "*",
		Command:   command,
		TimeoutMs: 5000,
	})

	entriesJSON, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshal postToolUse: %w", err)
	}
	hooks["postToolUse"] = entriesJSON

	hooksJSON, err := json.Marshal(hooks)
	if err != nil {
		return fmt.Errorf("marshal hooks: %w", err)
	}
	config["hooks"] = hooksJSON

	return writeKiroAgentConfig(agentPath, config)
}

// IsInstalled reports whether dp hooks are already configured in Kiro's
// agent config directory.
func (k *kiro) IsInstalled(configDir string) (bool, error) {
	if configDir == "" {
		dir, err := kiroSettingsDir()
		if err != nil {
			return false, err
		}
		configDir = filepath.Join(dir, "agents")
	}
	agentPath := filepath.Join(configDir, "dp-hooks.json")

	config, err := readKiroAgentConfig(agentPath)
	if err != nil {
		return false, err
	}

	raw, ok := config["hooks"]
	if !ok {
		return false, nil
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(raw, &hooks); err != nil {
		return false, fmt.Errorf("parse hooks: %w", err)
	}

	for _, eventRaw := range hooks {
		var entries []kiroHookEntry
		if err := json.Unmarshal(eventRaw, &entries); err != nil {
			continue
		}
		if hasKiroDPCommand(entries, dpKiroRecordCommand) || hasKiroDPCommand(entries, dpKiroIngestCommand) {
			return true, nil
		}
	}

	return false, nil
}

// readKiroAgentConfig reads and parses a Kiro agent config file,
// returning an empty map if the file does not exist.
func readKiroAgentConfig(path string) (kiroAgentConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(kiroAgentConfig), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var c kiroAgentConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, nil
}

// writeKiroAgentConfig writes the agent config to disk, creating the
// parent directory if needed.
func writeKiroAgentConfig(path string, config kiroAgentConfig) error {
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

// hasKiroDPCommand returns true if entries already contain a hook running
// the given command.
func hasKiroDPCommand(entries []kiroHookEntry, command string) bool {
	for _, e := range entries {
		if e.Command == command {
			return true
		}
	}
	return false
}
