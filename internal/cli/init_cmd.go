package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/scbrown/desire-path/internal/source"
	"github.com/spf13/cobra"
)

var (
	initSource     string
	initClaudeCode bool
	initTrackAll   bool
)

// initCmd configures integration with AI coding tools.
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Set up integration with AI coding tools",
	Long: `Init configures dp to automatically record failed tool calls from AI coding
assistants. Use --source to specify which tool to configure.

By default, only failures are recorded (PostToolUseFailure → dp record).
Use --track-all to also record every tool invocation via dp ingest. This
fires on every tool call and can generate significant data.

The command delegates to the source plugin's installer, which merges
configuration into the tool's settings file without overwriting existing
hooks or other configuration.`,
	Example: `  dp init --source claude-code
  dp init --source claude-code --track-all
  dp init --claude-code`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Handle deprecated --claude-code flag as alias.
		if initClaudeCode {
			if initSource != "" && initSource != "claude-code" {
				return fmt.Errorf("--claude-code conflicts with --source %s", initSource)
			}
			initSource = "claude-code"
		}

		if initSource == "" {
			names := source.Names()
			if len(names) == 0 {
				return fmt.Errorf("specify a source with --source NAME")
			}
			return fmt.Errorf("specify a source with --source NAME (available: %s)", strings.Join(names, ", "))
		}

		return runInit(initSource, initTrackAll)
	},
}

func init() {
	initCmd.Flags().StringVar(&initSource, "source", "", "source plugin to configure (e.g., claude-code)")
	initCmd.Flags().BoolVar(&initTrackAll, "track-all", false, "also install hooks to record all tool invocations (not just failures)")
	initCmd.Flags().BoolVar(&initClaudeCode, "claude-code", false, "configure Claude Code integration (deprecated: use --source claude-code)")
	initCmd.Flags().MarkDeprecated("claude-code", "use --source claude-code instead")
	rootCmd.AddCommand(initCmd)
}

// runInit looks up the named source plugin, checks if it supports
// installation, and delegates to its Install method.
func runInit(name string, trackAll bool) error {
	src := source.Get(name)
	if src == nil {
		names := source.Names()
		if len(names) == 0 {
			return fmt.Errorf("unknown source %q", name)
		}
		return fmt.Errorf("unknown source %q (available: %s)", name, strings.Join(names, ", "))
	}

	installer, ok := src.(source.Installer)
	if !ok {
		return fmt.Errorf("source %q does not support auto-install", name)
	}

	// Check if hooks are already configured for idempotency.
	installed, err := installer.IsInstalled("")
	if err == nil && installed {
		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]interface{}{
				"status": "already_configured",
				"source": name,
			})
		}
		fmt.Fprintf(os.Stdout, "hooks already configured for %s\n", name)
		return nil
	}

	opts := source.InstallOpts{TrackAll: trackAll}
	if err := installer.Install(opts); err != nil {
		return err
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]interface{}{
			"status":    "configured",
			"source":    name,
			"track_all": trackAll,
		})
	}
	fmt.Fprintf(os.Stdout, "Source %q integration configured!\n", name)
	if trackAll {
		fmt.Fprintf(os.Stdout, "All tool invocations will be recorded (PostToolUse + PostToolUseFailure).\n")
	} else {
		fmt.Fprintf(os.Stdout, "Failures will be automatically recorded when tools fail.\n")
	}
	fmt.Fprintf(os.Stdout, "View them with:   dp list\n")
	fmt.Fprintf(os.Stdout, "Analyze patterns: dp paths\n")
	return nil
}
