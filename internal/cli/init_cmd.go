package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var initClaudeCode bool

// initCmd configures integration with AI coding tools.
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Set up integration with AI coding tools",
	Long: `Init configures dp to automatically record failed tool calls from AI coding
assistants. Currently supports Claude Code via the --claude-code flag.

The command merges configuration into the tool's settings file without
overwriting existing hooks or other configuration.`,
	Example: `  dp init --claude-code`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !initClaudeCode {
			return fmt.Errorf("specify an integration flag (e.g., --claude-code)")
		}
		return setupClaudeCode()
	},
}

func init() {
	initCmd.Flags().BoolVar(&initClaudeCode, "claude-code", false, "configure Claude Code PostToolUseFailure hook")
	rootCmd.AddCommand(initCmd)
}

// claudeSettings represents the relevant subset of ~/.claude/settings.json.
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

// setupClaudeCode merges the PostToolUseFailure hook into ~/.claude/settings.json.
func setupClaudeCode() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("determine home directory: %w", err)
	}

	settingsPath := filepath.Join(home, ".claude", "settings.json")
	return setupClaudeCodeAt(settingsPath)
}

// setupClaudeCodeAt performs the Claude Code setup using the given settings path.
// Separated from setupClaudeCode for testing.
func setupClaudeCodeAt(settingsPath string) error {
	// Read existing settings or start fresh.
	settings, err := readClaudeSettings(settingsPath)
	if err != nil {
		return err
	}

	// Build the desired hook entry.
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

	// Merge PostToolUseFailure hook into existing hooks.
	hooks, err := mergeHook(settings, dpHook)
	if err != nil {
		return err
	}

	hooksJSON, err := json.Marshal(hooks)
	if err != nil {
		return fmt.Errorf("marshal hooks: %w", err)
	}
	settings["hooks"] = hooksJSON

	// Write back settings.
	if err := writeClaudeSettings(settingsPath, settings); err != nil {
		return err
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]string{
			"status":        "configured",
			"settings_path": settingsPath,
			"event":         "PostToolUseFailure",
			"command":       dpHookCommand,
		})
	}
	fmt.Fprintf(os.Stdout, "Claude Code integration configured!\n\n")
	fmt.Fprintf(os.Stdout, "Added hook to %s\n", settingsPath)
	fmt.Fprintf(os.Stdout, "  Event:   PostToolUseFailure\n")
	fmt.Fprintf(os.Stdout, "  Command: %s\n\n", dpHookCommand)
	fmt.Fprintf(os.Stdout, "Desires will be automatically recorded when Claude Code tools fail.\n")
	fmt.Fprintf(os.Stdout, "View them with:   dp list\n")
	fmt.Fprintf(os.Stdout, "Analyze patterns: dp paths\n")
	return nil
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

// mergeHook merges the dp hook into the PostToolUseFailure hook list without
// clobbering existing hooks.
func mergeHook(settings claudeSettings, dpHook claudeHookEntry) (map[string]json.RawMessage, error) {
	// Parse existing hooks map, or start fresh.
	hooks := make(map[string]json.RawMessage)
	if raw, ok := settings["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil {
			return nil, fmt.Errorf("parse existing hooks: %w", err)
		}
	}

	// Parse existing PostToolUseFailure entries.
	var entries []claudeHookEntry
	if raw, ok := hooks["PostToolUseFailure"]; ok {
		if err := json.Unmarshal(raw, &entries); err != nil {
			return nil, fmt.Errorf("parse PostToolUseFailure hooks: %w", err)
		}
	}

	// Check if dp hook is already present.
	if hasDPHook(entries) {
		fmt.Fprintf(os.Stderr, "Claude Code integration already configured.\n")
		fmt.Fprintf(os.Stderr, "  PostToolUseFailure hook for dp is already present in settings.\n")
		return hooks, nil
	}

	// Append dp hook entry.
	entries = append(entries, dpHook)
	entriesJSON, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("marshal PostToolUseFailure: %w", err)
	}
	hooks["PostToolUseFailure"] = entriesJSON

	return hooks, nil
}

// hasDPHook returns true if entries already contain a hook running dp record.
func hasDPHook(entries []claudeHookEntry) bool {
	for _, e := range entries {
		for _, h := range e.Hooks {
			if h.Command == dpHookCommand {
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
