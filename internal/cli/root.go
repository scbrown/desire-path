// Package cli defines the cobra command tree for the dp CLI.
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/scbrown/desire-path/internal/config"
	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

var (
	dbPath     string
	jsonOutput bool
	storeMode  string
	remoteURL  string
)

func defaultDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "desires.db"
	}
	return filepath.Join(home, ".dp", "desires.db")
}

// rootCmd is the top-level dp command.
var rootCmd = &cobra.Command{
	Use:   "dp",
	Short: "Desire Path - track and analyze failed AI tool calls",
	Long: `dp collects, analyzes, and surfaces patterns from failed AI tool calls.
Failed tool calls are signals that reveal capabilities the AI expects to exist.
By tracking these "desires", developers can implement features or aliases so
future similar attempts succeed.

Data is stored in a SQLite database at ~/.dp/desires.db (configurable via
--db flag or dp config db_path). All output commands support --json for
machine-readable output.`,
	Example: `  # Record a failed tool call
  echo '{"tool_name":"read_file","error":"unknown tool"}' | dp record

  # View recent desires and top patterns
  dp list --since 7d
  dp paths --top 10

  # Find similar known tools and create an alias
  dp suggest read_file
  dp alias read_file Read

  # Set up automatic recording from Claude Code
  dp init --claude-code`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadFrom(configPath)
		if err != nil {
			return
		}
		if cfg.DBPath != "" && !cmd.Flags().Changed("db") {
			dbPath = cfg.DBPath
		}
		if cfg.DefaultFormat == "json" && !cmd.Flags().Changed("json") {
			jsonOutput = true
		}
		if cfg.StoreMode != "" && storeMode == "" {
			storeMode = cfg.StoreMode
		}
		if cfg.RemoteURL != "" && remoteURL == "" {
			remoteURL = cfg.RemoteURL
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDBPath(), "path to SQLite database")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
}

// openStore returns a store.Store based on the current configuration.
// When store_mode is "remote", it returns a RemoteStore pointing at remote_url.
// Otherwise it opens the local SQLite database.
func openStore() (store.Store, error) {
	if storeMode == "remote" {
		if remoteURL == "" {
			return nil, fmt.Errorf("store_mode is \"remote\" but remote_url is not set; use: dp config set remote_url <url>")
		}
		return store.NewRemote(remoteURL), nil
	}
	return store.New(dbPath)
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
