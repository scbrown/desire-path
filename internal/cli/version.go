package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Version and Commit are set at build time via -ldflags.
//
//	go build -ldflags "-X github.com/scbrown/desire-path/internal/cli.Version=v0.2.0
//	  -X github.com/scbrown/desire-path/internal/cli.Commit=48cae1d"
var (
	Version = ""
	Commit  = ""
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version and commit hash",
	Long: `Print the dp version string.

When built from a tagged release, shows the release version.
Otherwise shows "dev". The git commit hash is always included.

Examples:
  dp v0.2.0 (48cae1d)
  dp dev (48cae1d)`,
	Run: func(cmd *cobra.Command, args []string) {
		v := Version
		if v == "" {
			v = "dev"
		}

		c := Commit
		if c == "" {
			c = commitFromBuildInfo()
		}

		if c != "" {
			fmt.Printf("dp %s (%s)\n", v, shortCommit(c))
		} else {
			fmt.Printf("dp %s\n", v)
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}

// commitFromBuildInfo extracts vcs.revision from Go's embedded build info.
func commitFromBuildInfo() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			return s.Value
		}
	}
	return ""
}

// shortCommit returns the first 7 characters of a commit hash.
func shortCommit(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}
