package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/scbrown/desire-path/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config [key] [value]",
	Short: "Show or modify configuration",
	Long: `View or change dp configuration stored in ~/.dp/config.toml.

With no arguments, shows all configuration settings.
With one argument, shows the value of that key.
With two arguments, sets the key to the given value.

Settings:
  db_path         Path to the SQLite database
  default_source  Default source tag for recorded desires
  known_tools     Comma-separated list of known tool names (for suggest)
  default_format  Default output format: "table" or "json"`,
	Example: `  dp config
  dp config db_path
  dp config db_path /custom/path/desires.db
  dp config default_source claude-code
  dp config known_tools Read,Write,Bash,Glob,Grep
  dp config default_format json`,
	Args: cobra.MaximumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadFrom(configPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		switch len(args) {
		case 0:
			return showConfig(cfg)
		case 1:
			return getConfig(cfg, args[0])
		default:
			return setConfig(cfg, args[0], args[1])
		}
	},
}

// configPath is the path to the config file, settable for testing.
var configPath = config.Path()

func init() {
	rootCmd.AddCommand(configCmd)
}

func showConfig(cfg *config.Config) error {
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "KEY\tVALUE")
	for _, key := range config.ValidKeys() {
		val, _ := cfg.Get(key)
		if val == "" {
			val = "(not set)"
		}
		fmt.Fprintf(w, "%s\t%s\n", key, val)
	}
	return w.Flush()
}

func getConfig(cfg *config.Config, key string) error {
	val, err := cfg.Get(key)
	if err != nil {
		return err
	}
	if val == "" {
		return nil
	}
	fmt.Println(val)
	return nil
}

func setConfig(cfg *config.Config, key, value string) error {
	if err := cfg.Set(key, value); err != nil {
		return err
	}
	if err := cfg.SaveTo(configPath); err != nil {
		return err
	}
	fmt.Printf("%s = %s\n", key, value)
	return nil
}
