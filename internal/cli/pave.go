package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/source"
	"github.com/spf13/cobra"
)

var (
	paveHook     bool
	paveAgentsMD bool
	paveAppend   string
	paveSettings string
)

// paveCmd turns alias data into actionable intercepts.
var paveCmd = &cobra.Command{
	Use:   "pave",
	Short: "Turn aliases into active tool-call intercepts",
	Long: `Pave makes aliases actionable. Two modes:

  --hook       Install a PreToolUse hook that intercepts hallucinated tool names
               in real time. When Claude tries to call a tool that has an alias,
               the hook blocks the call and tells Claude to use the correct name.

  --agents-md  Generate AGENTS.md / CLAUDE.md rules from alias data. These rules
               prevent hallucination proactively by telling Claude which tool names
               are wrong before it tries them. Output goes to stdout by default,
               or use --append to write directly to a file.

Belt and suspenders: --hook is reactive (catches mistakes), --agents-md is
preventive (stops them before they happen). Use both for maximum coverage.`,
	Example: `  # Install the PreToolUse intercept hook
  dp pave --hook

  # Generate AGENTS.md rules to stdout
  dp pave --agents-md

  # Append rules to an existing AGENTS.md file
  dp pave --agents-md --append AGENTS.md`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !paveHook && !paveAgentsMD {
			return fmt.Errorf("specify --hook or --agents-md (or both)")
		}
		if paveHook {
			if err := runPaveHook(); err != nil {
				return err
			}
		}
		if paveAgentsMD {
			if err := runPaveAgentsMD(); err != nil {
				return err
			}
		}
		return nil
	},
}

func init() {
	paveCmd.Flags().BoolVar(&paveHook, "hook", false, "install PreToolUse intercept hook")
	paveCmd.Flags().BoolVar(&paveAgentsMD, "agents-md", false, "generate AGENTS.md rules from aliases")
	paveCmd.Flags().StringVar(&paveAppend, "append", "", "append generated rules to this file (with --agents-md)")
	paveCmd.Flags().StringVar(&paveSettings, "settings", "", "path to settings file (default: ~/.claude/settings.json)")
	rootCmd.AddCommand(paveCmd)
}

// dpPaveCheckCommand is the command installed in the PreToolUse hook.
const dpPaveCheckCommand = "dp pave-check"

// runPaveHook installs a PreToolUse hook into ~/.claude/settings.json
// that runs dp pave-check on every tool call.
func runPaveHook() error {
	settingsPath := paveSettings
	if settingsPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("determine home directory: %w", err)
		}
		settingsPath = filepath.Join(home, ".claude", "settings.json")
	}

	settings, err := source.ReadClaudeSettings(settingsPath)
	if err != nil {
		return err
	}

	// Check if already installed.
	if source.HasDPHook(settings, "PreToolUse", dpPaveCheckCommand) {
		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]interface{}{
				"status": "already_configured",
				"hook":   "PreToolUse",
			})
		}
		fmt.Fprintln(os.Stdout, "PreToolUse hook already installed.")
		return nil
	}

	// Install the hook.
	if err := source.MergeClaudeHook(settings, "PreToolUse", dpPaveCheckCommand, 3000); err != nil {
		return err
	}
	if err := source.WriteClaudeSettings(settingsPath, settings); err != nil {
		return err
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]interface{}{
			"status": "configured",
			"hook":   "PreToolUse",
		})
	}
	fmt.Fprintln(os.Stdout, "PreToolUse intercept hook installed!")
	fmt.Fprintln(os.Stdout, "Hallucinated tool names matching aliases will now be blocked automatically.")
	fmt.Fprintln(os.Stdout, "Manage aliases with: dp alias <from> <to>")
	return nil
}

// runPaveAgentsMD generates AGENTS.md rules from alias data.
// Tool-name aliases get a "Tool Name Corrections" section.
// Command correction rules get a "Command Corrections" section grouped by command.
func runPaveAgentsMD() error {
	s, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	aliases, err := s.GetAliases(context.Background())
	if err != nil {
		return fmt.Errorf("get aliases: %w", err)
	}

	if len(aliases) == 0 {
		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]interface{}{
				"status": "no_aliases",
				"rules":  []string{},
			})
		}
		fmt.Fprintln(os.Stderr, "No aliases configured. Add some with: dp alias <from> <to>")
		return nil
	}

	// Split into tool-name aliases and command correction rules.
	var toolAliases, cmdRules []model.Alias
	for _, a := range aliases {
		if a.IsToolNameAlias() {
			toolAliases = append(toolAliases, a)
		} else {
			cmdRules = append(cmdRules, a)
		}
	}

	var sb strings.Builder

	// Tool name corrections section.
	if len(toolAliases) > 0 {
		sb.WriteString("# Tool Name Corrections\n\n")
		sb.WriteString("The following tool names are INCORRECT. Use the correct names instead:\n\n")
		for _, a := range toolAliases {
			sb.WriteString(fmt.Sprintf("- Do NOT call `%s`. Use `%s` instead.\n", a.From, a.To))
		}
		sb.WriteString("\n")
	}

	// Command corrections section, grouped by command.
	if len(cmdRules) > 0 {
		sb.WriteString("# Command Corrections\n\n")

		// Group rules by command (or tool:param for advanced rules).
		groups := make(map[string][]model.Alias)
		var order []string
		for _, r := range cmdRules {
			key := r.Command
			if key == "" {
				key = r.Tool + ":" + r.Param
			}
			if _, exists := groups[key]; !exists {
				order = append(order, key)
			}
			groups[key] = append(groups[key], r)
		}

		for _, key := range order {
			rules := groups[key]
			first := rules[0]

			// Section header varies by match kind.
			switch first.MatchKind {
			case "command":
				sb.WriteString(fmt.Sprintf("## %s → %s\n\n", first.From, first.To))
			case "flag", "literal":
				sb.WriteString(fmt.Sprintf("## %s\n\n", first.Command))
			default:
				if first.Command != "" {
					sb.WriteString(fmt.Sprintf("## %s\n\n", first.Command))
				} else {
					sb.WriteString(fmt.Sprintf("## %s (param: %s)\n\n", first.Tool, first.Param))
				}
			}

			for _, r := range rules {
				desc := formatRuleDescription(r)
				sb.WriteString("- " + desc + "\n")
			}
			sb.WriteString("\n")
		}
	}

	output := sb.String()

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		lines := make([]string, 0, len(aliases))
		for _, a := range toolAliases {
			lines = append(lines, fmt.Sprintf("Do NOT call `%s`. Use `%s` instead.", a.From, a.To))
		}
		for _, r := range cmdRules {
			lines = append(lines, formatRuleDescription(r))
		}
		return enc.Encode(map[string]interface{}{
			"status": "generated",
			"rules":  lines,
			"count":  len(aliases),
		})
	}

	if paveAppend != "" {
		f, err := os.OpenFile(paveAppend, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("open %s: %w", paveAppend, err)
		}
		defer f.Close()
		if _, err := f.WriteString("\n" + output); err != nil {
			return fmt.Errorf("write %s: %w", paveAppend, err)
		}
		fmt.Fprintf(os.Stdout, "Appended %d rules to %s\n", len(aliases), paveAppend)
		return nil
	}

	fmt.Print(output)
	return nil
}

// formatRuleDescription returns a human-readable description for a command correction rule.
func formatRuleDescription(r model.Alias) string {
	var desc string
	switch r.MatchKind {
	case "flag":
		desc = fmt.Sprintf("Flag `-%s` should be `-%s`", r.From, r.To)
	case "command":
		desc = fmt.Sprintf("Use `%s` instead of `%s`", r.To, r.From)
	case "literal":
		desc = fmt.Sprintf("`%s` → `%s`", r.From, r.To)
	case "regex":
		desc = fmt.Sprintf("Pattern `%s` → `%s`", r.From, r.To)
	case "recipe":
		if r.Message != "" {
			desc = fmt.Sprintf("Do NOT use `%s`. %s", r.From, r.Message)
		} else {
			desc = fmt.Sprintf("Do NOT use `%s` — it does not exist and will be rewritten automatically.", r.From)
		}
	default:
		desc = fmt.Sprintf("`%s` → `%s`", r.From, r.To)
	}
	if r.Message != "" {
		desc += " (" + r.Message + ")"
	}
	return desc
}
