package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// knownCodexNotifyFields lists JSON keys from the Codex notify hook payload
// that map to universal Fields. Everything else goes into Extra.
var knownCodexNotifyFields = map[string]bool{
	"cwd": true,
}

// codexCLI implements Source for OpenAI Codex CLI.
type codexCLI struct{}

func init() {
	Register(&codexCLI{})
}

// Name returns "codex".
func (c *codexCLI) Name() string { return "codex" }

// Description returns a short human-readable description of this source.
func (c *codexCLI) Description() string { return "OpenAI Codex CLI notify hook and exec events" }

// Extract parses Codex CLI JSON and maps fields to the universal Fields struct.
// It handles two formats:
//
//  1. Notify hook payload: {"type":"agent-turn-complete","thread-id":"...","cwd":"..."}
//  2. Item event from codex exec --json: {"type":"item.completed","item":{"type":"command_execution",...}}
//
// For notify payloads, ToolName is set to "agent_turn" (a synthetic tool name).
// For item events, ToolName is set to the item's type (e.g., "command_execution").
func (c *codexCLI) Extract(raw []byte) (*Fields, error) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("codex: parsing JSON: %w", err)
	}

	// Detect format by "type" field.
	typeRaw, ok := m["type"]
	if !ok {
		return nil, fmt.Errorf("codex: missing required field: type")
	}

	var eventType string
	if err := json.Unmarshal(typeRaw, &eventType); err != nil {
		return nil, fmt.Errorf("codex: parsing type: %w", err)
	}
	if eventType == "" {
		return nil, fmt.Errorf("codex: missing required field: type")
	}

	if eventType == "agent-turn-complete" {
		return c.extractNotify(m, eventType)
	}

	if eventType == "item.completed" || eventType == "item.started" {
		return c.extractItem(m, eventType)
	}

	return nil, fmt.Errorf("codex: unsupported event type: %q", eventType)
}

// extractNotify handles the Codex notify hook payload format.
func (c *codexCLI) extractNotify(m map[string]json.RawMessage, eventType string) (*Fields, error) {
	var f Fields
	f.ToolName = "agent_turn"

	// thread-id → InstanceID
	if v, ok := m["thread-id"]; ok {
		if err := json.Unmarshal(v, &f.InstanceID); err != nil {
			return nil, fmt.Errorf("codex: parsing thread-id: %w", err)
		}
	}

	// cwd → CWD
	if v, ok := m["cwd"]; ok {
		if err := json.Unmarshal(v, &f.CWD); err != nil {
			return nil, fmt.Errorf("codex: parsing cwd: %w", err)
		}
	}

	// Collect non-universal fields into Extra.
	extra := make(map[string]json.RawMessage)
	for k, v := range m {
		if !knownCodexNotifyFields[k] && k != "thread-id" && k != "type" {
			extra[k] = v
		}
	}
	// Always include event_type in Extra for downstream consumers.
	etJSON, _ := json.Marshal(eventType)
	extra["event_type"] = etJSON

	if len(extra) > 0 {
		f.Extra = extra
	}

	return &f, nil
}

// extractItem handles the codex exec --json item event format.
func (c *codexCLI) extractItem(m map[string]json.RawMessage, eventType string) (*Fields, error) {
	itemRaw, ok := m["item"]
	if !ok {
		return nil, fmt.Errorf("codex: %s event missing required field: item", eventType)
	}

	var item map[string]json.RawMessage
	if err := json.Unmarshal(itemRaw, &item); err != nil {
		return nil, fmt.Errorf("codex: parsing item: %w", err)
	}

	var f Fields

	// item.type → ToolName (e.g., "command_execution", "file_change")
	itemTypeRaw, ok := item["type"]
	if !ok {
		return nil, fmt.Errorf("codex: item missing required field: type")
	}
	if err := json.Unmarshal(itemTypeRaw, &f.ToolName); err != nil {
		return nil, fmt.Errorf("codex: parsing item type: %w", err)
	}
	if f.ToolName == "" {
		return nil, fmt.Errorf("codex: item missing required field: type")
	}

	// item.command → ToolInput for command_execution items
	if cmd, ok := item["command"]; ok {
		f.ToolInput = cmd
	}

	// item.status → Error if status indicates failure
	if statusRaw, ok := item["status"]; ok {
		var status string
		if err := json.Unmarshal(statusRaw, &status); err == nil {
			if status == "failed" || status == "error" {
				f.Error = "item " + status
			}
		}
	}

	// Collect item-level fields (except "type") into Extra.
	extra := make(map[string]json.RawMessage)
	for k, v := range item {
		if k != "type" && k != "command" && k != "status" {
			extra[k] = v
		}
	}

	// Include top-level event fields.
	etJSON, _ := json.Marshal(eventType)
	extra["event_type"] = etJSON

	// Include item status in Extra even though we derived Error from it.
	if statusRaw, ok := item["status"]; ok {
		extra["status"] = statusRaw
	}

	if len(extra) > 0 {
		f.Extra = extra
	}

	return &f, nil
}

// dpCodexNotifyCommand is the shell script command installed as the notify hook.
// Codex passes the JSON payload as a single CLI argument to the notify command,
// so we use a shell wrapper to pipe it to dp ingest on stdin.
const dpCodexNotifyCommand = `printf '%s' "$1" | dp ingest --source codex`

// dpCodexNotifyScript is the full notify config value: a bash invocation that
// passes the argument to dp ingest.
var dpCodexNotifyScript = []string{"bash", "-c", dpCodexNotifyCommand, "--"}

// Install configures the Codex CLI notify hook. It modifies ~/.codex/config.toml
// to add a notify entry that pipes the turn-complete JSON to dp ingest.
func (c *codexCLI) Install(opts InstallOpts) error {
	configPath := opts.SettingsPath
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("determine home directory: %w", err)
		}
		configPath = filepath.Join(home, ".codex", "config.toml")
	}
	return installCodexNotify(configPath)
}

// IsInstalled checks whether a dp notify hook is already configured in the
// Codex CLI config file. configDir should point to the directory containing
// config.toml (e.g., ~/.codex).
func (c *codexCLI) IsInstalled(configDir string) (bool, error) {
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false, fmt.Errorf("determine home directory: %w", err)
		}
		configDir = filepath.Join(home, ".codex")
	}
	configPath := filepath.Join(configDir, "config.toml")

	config, err := readCodexConfig(configPath)
	if err != nil {
		return false, err
	}

	notify, ok := config["notify"]
	if !ok {
		return false, nil
	}

	// notify should be an array of strings.
	notifyArr, ok := notify.([]interface{})
	if !ok {
		return false, nil
	}

	return codexNotifyHasDPCommand(notifyArr), nil
}

// installCodexNotify performs the Codex CLI setup using the given config path.
func installCodexNotify(configPath string) error {
	config, err := readCodexConfig(configPath)
	if err != nil {
		return err
	}

	// Check if already installed.
	if existing, ok := config["notify"]; ok {
		if arr, ok := existing.([]interface{}); ok {
			if codexNotifyHasDPCommand(arr) {
				return nil // Already installed.
			}
		}
		// A non-dp notify is already configured. Codex only supports a single
		// notify command, so we can't just append. Return an error.
		return fmt.Errorf("codex: existing notify configuration found; "+
			"Codex supports only one notify command. Please manually update %s "+
			"to include dp ingest", configPath)
	}

	config["notify"] = dpCodexNotifyScript

	return writeCodexConfig(configPath, config)
}

// readCodexConfig reads and parses a Codex TOML config file, returning an
// empty map if the file does not exist.
func readCodexConfig(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return make(map[string]interface{}), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var config map[string]interface{}
	if err := toml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return config, nil
}

// writeCodexConfig writes config back to the TOML file, creating the parent
// directory if needed.
func writeCodexConfig(path string, config map[string]interface{}) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	data, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// codexNotifyHasDPCommand checks if a notify array contains a dp ingest command.
func codexNotifyHasDPCommand(arr []interface{}) bool {
	for _, v := range arr {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if strings.Contains(s, "dp ingest") || strings.Contains(s, "dp record") {
			return true
		}
	}
	return false
}
