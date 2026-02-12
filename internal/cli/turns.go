package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/scbrown/desire-path/internal/config"
	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

var (
	turnsMinLength int
	turnsSince     string
	turnsSession   string
	turnsPatterns  bool
	turnsPattern   string
)

var turnsCmd = &cobra.Command{
	Use:   "turns",
	Short: "Show turn-level tool call patterns",
	Long: `Turns displays turn-level data from transcript analysis. A "turn" is one
human→model→human cycle in an AI session. Long turns (many tool calls) signal
that the agent's intent didn't map cleanly to available tools.

By default, shows turns exceeding the configured threshold (default 5).
Use --patterns to see clustered abstract patterns instead.`,
	Example: `  dp turns
  dp turns --min-length 3
  dp turns --patterns
  dp turns --pattern "Grep → Read{2+} → Edit"
  dp turns --since 2026-02-01T00:00:00Z
  dp turns --session abc123
  dp turns --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		minLen := turnsMinLength
		if minLen == 0 {
			cfg, cfgErr := config.LoadFrom(configPath)
			if cfgErr == nil && cfg.TurnLengthThreshold > 0 {
				minLen = cfg.TurnLengthThreshold
			} else {
				minLen = config.DefaultTurnLengthThreshold
			}
		}

		opts := store.TurnOpts{
			MinLength: minLen,
			SessionID: turnsSession,
			Pattern:   turnsPattern,
		}
		if turnsSince != "" {
			t, err := time.Parse(time.RFC3339, turnsSince)
			if err != nil {
				return fmt.Errorf("parse --since: %w", err)
			}
			opts.Since = t
		}

		if turnsPatterns {
			return runTurnsPatterns(s, opts)
		}
		return runTurnsList(s, opts)
	},
}

func init() {
	turnsCmd.Flags().IntVar(&turnsMinLength, "min-length", 0, "minimum tool calls per turn (0 = use config, default 5)")
	turnsCmd.Flags().StringVar(&turnsSince, "since", "", "only include turns after this time (RFC3339)")
	turnsCmd.Flags().StringVar(&turnsSession, "session", "", "filter by session ID")
	turnsCmd.Flags().BoolVar(&turnsPatterns, "patterns", false, "show clustered abstract patterns instead of individual turns")
	turnsCmd.Flags().StringVar(&turnsPattern, "pattern", "", "drill down to turns matching this abstract pattern")
	rootCmd.AddCommand(turnsCmd)
}

func runTurnsList(s store.Store, opts store.TurnOpts) error {
	turns, err := s.ListTurns(context.Background(), opts)
	if err != nil {
		return fmt.Errorf("list turns: %w", err)
	}

	if jsonOutput {
		return writeTurnsJSON(os.Stdout, turns)
	}
	writeTurnsTable(os.Stdout, turns)
	return nil
}

func runTurnsPatterns(s store.Store, opts store.TurnOpts) error {
	patterns, err := s.TurnPatternStats(context.Background(), opts)
	if err != nil {
		return fmt.Errorf("turn patterns: %w", err)
	}

	if jsonOutput {
		return writeTurnPatternsJSON(os.Stdout, patterns)
	}
	writeTurnPatternsTable(os.Stdout, patterns)
	return nil
}

func writeTurnsJSON(w io.Writer, turns []store.TurnRow) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if turns == nil {
		turns = []store.TurnRow{}
	}
	return enc.Encode(turns)
}

func writeTurnsTable(w io.Writer, turns []store.TurnRow) {
	tbl := NewTable(w, "SESSION", "TURN", "LENGTH", "TOOLS")
	for _, t := range turns {
		// Extract turn index from turn_id (format: "session:index").
		turnIdx := t.TurnID
		if idx := lastColon(t.TurnID); idx >= 0 {
			turnIdx = t.TurnID[idx+1:]
		}
		tbl.Row(
			truncate(t.SessionID, 12),
			turnIdx,
			fmt.Sprintf("%d", t.Length),
			t.Tools,
		)
	}
	tbl.Flush()
}

func writeTurnPatternsJSON(w io.Writer, patterns []store.TurnPattern) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if patterns == nil {
		patterns = []store.TurnPattern{}
	}
	return enc.Encode(patterns)
}

func writeTurnPatternsTable(w io.Writer, patterns []store.TurnPattern) {
	tbl := NewTable(w, "PATTERN", "COUNT", "AVG_LENGTH", "SESSIONS")
	for _, p := range patterns {
		tbl.Row(
			p.Pattern,
			fmt.Sprintf("%d", p.Count),
			fmt.Sprintf("%.1f", p.AvgLength),
			fmt.Sprintf("%d", p.Sessions),
		)
	}
	tbl.Flush()
}

// lastColon returns the index of the last ':' in s, or -1.
func lastColon(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ':' {
			return i
		}
	}
	return -1
}
