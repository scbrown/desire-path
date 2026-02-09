package cli

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/scbrown/desire-path/internal/source"
	"github.com/spf13/cobra"
)

// sourceInfo is the JSON structure for the sources command output.
type sourceInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Installer   bool   `json:"installer"`
	Installed   *bool  `json:"installed"` // nil when not an installer
}

// defaultConfigDir returns the default config directory for known source
// plugins. Returns "" for unknown sources.
func defaultConfigDir(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch name {
	case "claude-code":
		return filepath.Join(home, ".claude")
	default:
		return ""
	}
}

var sourcesCmd = &cobra.Command{
	Use:   "sources",
	Short: "List available source plugins",
	Long: `Display all registered source plugins showing their name, description,
whether they support auto-install, and current installation status.`,
	Example: `  dp sources
  dp sources --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listSources()
	},
}

func init() {
	rootCmd.AddCommand(sourcesCmd)
}

func listSources() error {
	names := source.Names()
	sources := make([]sourceInfo, 0, len(names))

	for _, name := range names {
		src := source.Get(name)
		info := sourceInfo{
			Name:        src.Name(),
			Description: src.Description(),
		}

		if inst, ok := src.(source.Installer); ok {
			info.Installer = true
			if dir := defaultConfigDir(name); dir != "" {
				installed, err := inst.IsInstalled(dir)
				if err == nil {
					info.Installed = &installed
				}
			}
		}

		sources = append(sources, info)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(sources)
	}

	tbl := NewTable(os.Stdout, "NAME", "DESCRIPTION", "INSTALLER", "INSTALLED")
	for _, s := range sources {
		installer := "no"
		if s.Installer {
			installer = "yes"
		}
		installed := "-"
		if s.Installed != nil {
			if *s.Installed {
				installed = "yes"
			} else {
				installed = "no"
			}
		}
		tbl.Row(s.Name, s.Description, installer, installed)
	}
	return tbl.Flush()
}
