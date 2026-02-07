// Package cli defines the cobra command tree for the dp CLI.
package cli

import (
	"os"
	"path/filepath"

	"github.com/scbrown/desire-path/internal/config"
	"github.com/spf13/cobra"
)

var (
	dbPath     string
	jsonOutput bool
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
future similar attempts succeed.`,
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
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDBPath(), "path to SQLite database")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
