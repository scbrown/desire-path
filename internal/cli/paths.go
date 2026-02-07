package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

var (
	pathsTop   int
	pathsSince string
)

// pathsCmd displays aggregated desire paths ranked by frequency.
var pathsCmd = &cobra.Command{
	Use:   "paths",
	Short: "Show aggregated paths ranked by frequency",
	Long: `Paths displays aggregated desire patterns ranked by how often they occur.
Each row represents a unique tool name that has been recorded as a failed call,
along with its frequency count, first/last occurrence, and any configured alias.`,
	Example: `  dp paths
  dp paths --top 10
  dp paths --since 2026-02-01T00:00:00Z
  dp paths --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer s.Close()

		opts := store.PathOpts{Top: pathsTop}
		if pathsSince != "" {
			t, err := time.Parse(time.RFC3339, pathsSince)
			if err != nil {
				return fmt.Errorf("parse --since: %w", err)
			}
			opts.Since = t
		}

		paths, err := s.GetPaths(context.Background(), opts)
		if err != nil {
			return fmt.Errorf("get paths: %w", err)
		}

		if jsonOutput {
			return writePathsJSON(os.Stdout, paths)
		}
		writePathsTable(os.Stdout, paths)
		return nil
	},
}

func init() {
	pathsCmd.Flags().IntVar(&pathsTop, "top", 20, "maximum number of paths to display")
	pathsCmd.Flags().StringVar(&pathsSince, "since", "", "only include desires after this time (RFC3339)")
	rootCmd.AddCommand(pathsCmd)
}

// writePathsJSON writes paths as a JSON array to w.
func writePathsJSON(w io.Writer, paths []model.Path) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if paths == nil {
		paths = []model.Path{}
	}
	return enc.Encode(paths)
}

// writePathsTable writes paths as an aligned text table to w.
func writePathsTable(w io.Writer, paths []model.Path) {
	tbl := NewTable(w, "RANK", "PATTERN", "COUNT", "FIRST_SEEN", "LAST_SEEN", "ALIAS")
	for i, p := range paths {
		tbl.Row(
			fmt.Sprintf("%d", i+1),
			p.Pattern,
			fmt.Sprintf("%d", p.Count),
			p.FirstSeen.UTC().Format(time.RFC3339),
			p.LastSeen.UTC().Format(time.RFC3339),
			p.AliasTo,
		)
	}
	tbl.Flush()
}
