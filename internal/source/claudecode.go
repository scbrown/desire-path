package source

import (
	"bytes"
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

// Description returns a short human-readable description of this source.
func (c *claudeCode) Description() string { return "Claude Code PostToolUse/Failure hooks" }

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

// Install configures Claude Code hooks. It installs dp ingest hooks on
// both PostToolUse and PostToolUseFailure to record all invocations.
// The dual-write in the ingest pipeline ensures failures also appear
// in the desires table. opts.TrackAll is accepted but ignored (all
// invocations are always tracked).
func (c *claudeCode) Install(opts InstallOpts) error {
	settingsPath := opts.SettingsPath
	if settingsPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("determine home directory: %w", err)
		}
		settingsPath = filepath.Join(home, ".claude", "settings.json")
	}
	return installClaudeHooks(settingsPath, opts.Env)
}

// IsInstalled checks whether dp hooks are already configured in the Claude
// Code settings file at configDir/settings.json. If configDir is empty, it
// defaults to ~/.claude.
func (c *claudeCode) IsInstalled(configDir string) (bool, error) {
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false, fmt.Errorf("determine home directory: %w", err)
		}
		configDir = filepath.Join(home, ".claude")
	}
	settingsPath := filepath.Join(configDir, "settings.json")

	settings, err := readClaudeSettings(settingsPath)
	if err != nil {
		return false, err
	}

	raw, ok := settings["hooks"]
	if !ok {
		return false, nil
	}

	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(raw, &hooks); err != nil {
		return false, fmt.Errorf("parse hooks: %w", err)
	}

	var preUse, postUse, postFailure []claudeHookEntry
	if raw, ok := hooks["PreToolUse"]; ok {
		_ = json.Unmarshal(raw, &preUse)
	}
	if raw, ok := hooks["PostToolUse"]; ok {
		_ = json.Unmarshal(raw, &postUse)
	}
	if raw, ok := hooks["PostToolUseFailure"]; ok {
		_ = json.Unmarshal(raw, &postFailure)
	}
	// A prior dp installation is not current until the sibling signposting
	// hook is present too; this lets `dp init` converge upgrades.
	return hasDPHookCommand(postUse, dpHookCommand) &&
		hasDPHookCommand(postUse, dpSignpostCommand) &&
		hasDPHookCommand(preUse, dpSignpostPrefetchCommand) &&
		hasDPHookCommand(postFailure, dpHookCommand) &&
		hasDPHookCommand(postFailure, dpSignpostCommand) &&
		hasDPHookCommand(postFailure, dpPaveCorrectCommand), nil
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
	Type          string `json:"type"`
	Command       string `json:"command"`
	Timeout       int    `json:"timeout,omitempty"`
	StatusMessage string `json:"statusMessage,omitempty"`
}

// dpHookCommand is the canonical command installed for both hook events.
const dpHookCommand = "dp ingest --source claude-code"

// dpSignpostCommand is a sibling hook on BOTH PostToolUse and
// PostToolUseFailure. It never wraps the invoked command; it observes completed
// Bash searches and may emit disjoint context.
//
// The failure registration is not defensive, it is load-bearing: a literal
// search that finds NOTHING exits non-zero, so the null predicate — "your
// search found nothing, try a semantic one", the most valuable trigger
// signposting has — is routed to PostToolUseFailure and was unreachable on
// every crew pane while the hook lived only on PostToolUse. Measured with the
// identical query run twice, bare (exit 1) and with `|| true` (exit 0): one
// event, from the exit-0 run.
const dpSignpostCommand = "dp signpost"
const dpSignpostPrefetchCommand = "dp signpost-prefetch"

// dpPaveCorrectCommand is the PostToolUseFailure sibling. It is the correction
// half of the same contract the signpost keeps: it emits only when a stored rule
// matches the failure, it never modifies the failing tool call, and it stays
// silent otherwise. Re-EXECUTING the corrected command is gated separately, on
// the auto_correct config key, which is off by default.
//
// It rides the managed hook source rather than a hand-edited settings file:
// `gt hooks sync` deletes hooks it does not know about, so a hand-installed
// entry is removed on the next sync without telling anyone (aegis-uk7x).
const dpPaveCorrectCommand = "dp pave-correct"

// dpLegacyHookCommand is the old command that may exist in user settings.
// IsInstalled checks for both so upgrades are detected correctly.
const dpLegacyHookCommand = "dp record --source claude-code"

// installClaudeHooks performs the Claude Code setup using the given settings path.
// It installs dp ingest for both PostToolUse and PostToolUseFailure, the
// Bash-scoped signposting prefetch/gating siblings, and the pave-correct
// sibling on PostToolUseFailure. The dual-write in the ingest
// pipeline ensures failures appear in both invocations and desires tables.
func installClaudeHooks(settingsPath string, env map[string]string) error {
	settings, err := readClaudeSettings(settingsPath)
	if err != nil {
		return err
	}
	if err := mergeSettingsEnv(settings, env); err != nil {
		return err
	}

	type hookDef struct {
		event   string
		command string
	}
	defs := []hookDef{
		{"PreToolUse", dpSignpostPrefetchCommand},
		{"PostToolUse", dpHookCommand},
		{"PostToolUseFailure", dpHookCommand},
		{"PostToolUse", dpSignpostCommand},
		{"PostToolUseFailure", dpSignpostCommand},
		{"PostToolUseFailure", dpPaveCorrectCommand},
	}

	for _, d := range defs {
		matcher := ".*"
		if d.command == dpSignpostCommand || d.command == dpSignpostPrefetchCommand {
			matcher = "Bash"
		}
		hooks, err := mergeHookEvent(settings, d.event, claudeHookEntry{
			Matcher: matcher,
			Hooks: []claudeHookInner{
				{Type: "command", Command: d.command, Timeout: 5000},
			},
		})
		if err != nil {
			return err
		}
		hooksJSON, err := marshalJSONNoEscape(hooks)
		if err != nil {
			return fmt.Errorf("marshal hooks: %w", err)
		}
		settings["hooks"] = hooksJSON
	}

	return writeClaudeSettings(settingsPath, settings)
}

// mergeSettingsEnv merges env into the settings' "env" object without
// disturbing keys it did not write. The environment a hook needs travels with
// the hook: installing one without the other is what produced a fleet-wide
// signposting hook that reached nothing for weeks.
func mergeSettingsEnv(settings claudeSettings, env map[string]string) error {
	if len(env) == 0 {
		return nil
	}
	existing := map[string]string{}
	if raw, ok := settings["env"]; ok && len(raw) > 0 {
		if err := json.Unmarshal(raw, &existing); err != nil {
			return fmt.Errorf("parse settings env: %w", err)
		}
	}
	for k, v := range env {
		existing[k] = v
	}
	encoded, err := marshalJSONNoEscape(existing)
	if err != nil {
		return fmt.Errorf("marshal settings env: %w", err)
	}
	settings["env"] = encoded
	return nil
}

// ReadClaudeSettings reads and parses the settings file, returning an empty
// map if the file does not exist.
func ReadClaudeSettings(path string) (claudeSettings, error) {
	return readClaudeSettings(path)
}

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
	entriesJSON, err := marshalJSONNoEscape(entries)
	if err != nil {
		return nil, fmt.Errorf("marshal %s: %w", eventName, err)
	}
	hooks[eventName] = entriesJSON

	return hooks, nil
}

// HasDPHook checks whether a specific hook command is installed for the given
// event in the parsed settings.
func HasDPHook(settings claudeSettings, event, command string) bool {
	raw, ok := settings["hooks"]
	if !ok {
		return false
	}
	var hooks map[string]json.RawMessage
	if err := json.Unmarshal(raw, &hooks); err != nil {
		return false
	}
	eventRaw, ok := hooks[event]
	if !ok {
		return false
	}
	var entries []claudeHookEntry
	if err := json.Unmarshal(eventRaw, &entries); err != nil {
		return false
	}
	return hasDPHookCommand(entries, command)
}

// MergeClaudeHook adds a hook command for the given event into settings,
// without clobbering existing hooks. The settings map is modified in place.
func MergeClaudeHook(settings claudeSettings, event, command string, timeout int) error {
	hooks, err := mergeHookEvent(settings, event, claudeHookEntry{
		Matcher: ".*",
		Hooks: []claudeHookInner{
			{Type: "command", Command: command, Timeout: timeout},
		},
	})
	if err != nil {
		return err
	}
	hooksJSON, err := marshalJSONNoEscape(hooks)
	if err != nil {
		return fmt.Errorf("marshal hooks: %w", err)
	}
	settings["hooks"] = hooksJSON
	return nil
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

// WriteClaudeSettings writes settings back to the file, creating the parent
// directory if needed.
func WriteClaudeSettings(path string, settings claudeSettings) error {
	return writeClaudeSettings(path, settings)
}

func writeClaudeSettings(path string, settings claudeSettings) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	data, err := marshalJSONNoEscape(settings)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// marshalJSONNoEscape marshals v as indented JSON without HTML-escaping
// characters like &, <, >. This preserves && in shell commands.
func marshalJSONNoEscape(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
