package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/scbrown/desire-path/internal/config"
	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

var (
	pathsTop   int
	pathsSince string
	pathsTurns bool
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
  dp paths --turns
  dp paths --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		opts := store.PathOpts{Top: pathsTop}
		var since time.Time
		if pathsSince != "" {
			t, err := time.Parse(time.RFC3339, pathsSince)
			if err != nil {
				return fmt.Errorf("parse --since: %w", err)
			}
			since = t
			opts.Since = since
		}

		paths, err := s.GetPaths(context.Background(), opts)
		if err != nil {
			return fmt.Errorf("get paths: %w", err)
		}

		if pathsTurns {
			threshold := config.DefaultTurnLengthThreshold
			cfg, cfgErr := config.LoadFrom(configPath)
			if cfgErr == nil {
				threshold = cfg.EffectiveTurnLengthThreshold()
			}
			turnStats, err := s.GetPathTurnStats(context.Background(), threshold, since)
			if err != nil {
				return fmt.Errorf("get turn stats: %w", err)
			}
			if jsonOutput {
				return writePathsTurnsJSON(os.Stdout, paths, turnStats)
			}
			writePathsTurnsTable(os.Stdout, paths, turnStats)
			return nil
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
	pathsCmd.Flags().BoolVar(&pathsTurns, "turns", false, "include turn context columns (AVG_TURN_LEN, LONG_TURN_%)")
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

// writePathsTurnsTable writes paths with turn statistics as an aligned text table.
func writePathsTurnsTable(w io.Writer, paths []model.Path, turnStats []store.ToolTurnStats) {
	statsMap := make(map[string]store.ToolTurnStats, len(turnStats))
	for _, ts := range turnStats {
		statsMap[ts.ToolName] = ts
	}
	tbl := NewTable(w, "RANK", "PATTERN", "COUNT", "AVG_TURN_LEN", "LONG_TURN_%", "ALIAS")
	for i, p := range paths {
		ts := statsMap[p.Pattern]
		avgStr := "-"
		pctStr := "-"
		if ts.ToolName != "" {
			avgStr = fmt.Sprintf("%.1f", ts.AvgTurnLen)
			pctStr = fmt.Sprintf("%.0f%%", ts.LongTurnPct)
		}
		tbl.Row(
			fmt.Sprintf("%d", i+1),
			p.Pattern,
			fmt.Sprintf("%d", p.Count),
			avgStr,
			pctStr,
			p.AliasTo,
		)
	}
	tbl.Flush()
}

// pathTurnJSON combines path data with turn statistics for JSON output.
type pathTurnJSON struct {
	model.Path
	AvgTurnLen  *float64 `json:"avg_turn_len,omitempty"`
	LongTurnPct *float64 `json:"long_turn_pct,omitempty"`
}

// writePathsTurnsJSON writes paths with turn statistics as JSON.
func writePathsTurnsJSON(w io.Writer, paths []model.Path, turnStats []store.ToolTurnStats) error {
	statsMap := make(map[string]store.ToolTurnStats, len(turnStats))
	for _, ts := range turnStats {
		statsMap[ts.ToolName] = ts
	}

	result := make([]pathTurnJSON, len(paths))
	for i, p := range paths {
		result[i] = pathTurnJSON{Path: p}
		if ts, ok := statsMap[p.Pattern]; ok {
			result[i].AvgTurnLen = &ts.AvgTurnLen
			result[i].LongTurnPct = &ts.LongTurnPct
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
