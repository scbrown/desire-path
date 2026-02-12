package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
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
	Short: "Show turns exceeding the configured length threshold",
	Long: `Turns displays turn-level data from tool invocations. A "turn" is one
human→model→human cycle. Turns with many tool calls indicate the agent
needed multiple attempts to accomplish a goal — a signal that tools aren't
getting agents where they need to go.

Use --patterns to cluster similar turns by abstract pattern, where
consecutive repeats of the same tool collapse (e.g., Read, Read, Read
becomes Read{2+}).

Use --pattern to drill down into a specific abstract pattern.`,
	Example: `  dp turns
  dp turns --min-length 8
  dp turns --since 2026-02-01T00:00:00Z
  dp turns --session abc123
  dp turns --patterns
  dp turns --pattern "Grep → Read{2+} → Edit"
  dp turns --json`,
	RunE: runTurns,
}

func init() {
	turnsCmd.Flags().IntVar(&turnsMinLength, "min-length", 0, "minimum turn length (default: turn_length_threshold config)")
	turnsCmd.Flags().StringVar(&turnsSince, "since", "", "only include turns after this time (RFC3339)")
	turnsCmd.Flags().StringVar(&turnsSession, "session", "", "filter by session ID")
	turnsCmd.Flags().BoolVar(&turnsPatterns, "patterns", false, "cluster similar turns by abstract pattern")
	turnsCmd.Flags().StringVar(&turnsPattern, "pattern", "", "drill down to turns matching an abstract pattern")
	rootCmd.AddCommand(turnsCmd)
}

func runTurns(cmd *cobra.Command, args []string) error {
	s, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	// Resolve min-length from config if not set via flag.
	minLen := turnsMinLength
	if minLen == 0 {
		cfg, cfgErr := config.LoadFrom(configPath)
		if cfgErr == nil {
			minLen = cfg.EffectiveTurnLengthThreshold()
		} else {
			minLen = config.DefaultTurnLengthThreshold
		}
	}

	opts := store.TurnOpts{MinLength: minLen, SessionID: turnsSession}
	if turnsSince != "" {
		t, err := time.Parse(time.RFC3339, turnsSince)
		if err != nil {
			return fmt.Errorf("parse --since: %w", err)
		}
		opts.Since = t
	}

	turns, err := s.GetTurns(context.Background(), opts)
	if err != nil {
		return fmt.Errorf("get turns: %w", err)
	}

	if turnsPatterns {
		return outputPatterns(os.Stdout, turns)
	}
	if turnsPattern != "" {
		return outputPatternDrilldown(os.Stdout, turns, turnsPattern)
	}

	if jsonOutput {
		return writeTurnsJSON(os.Stdout, turns)
	}
	writeTurnsTable(os.Stdout, turns)
	return nil
}

// writeTurnsJSON writes turns as a JSON array to w.
func writeTurnsJSON(w io.Writer, turns []store.TurnRow) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if turns == nil {
		turns = []store.TurnRow{}
	}
	return enc.Encode(turns)
}

// writeTurnsTable writes turns as an aligned text table to w.
func writeTurnsTable(w io.Writer, turns []store.TurnRow) {
	tbl := NewTable(w, "SESSION", "TURN", "LENGTH", "TOOLS")
	for _, t := range turns {
		session := t.SessionID
		if len(session) > 8 {
			session = session[:8]
		}
		tbl.Row(
			session,
			fmt.Sprintf("%d", t.TurnIndex),
			fmt.Sprintf("%d", t.Length),
			strings.Join(t.Tools, " → "),
		)
	}
	tbl.Flush()
}

// abstractPattern collapses consecutive repeats of the same tool into Tool{2+}.
func abstractPattern(tools []string) string {
	if len(tools) == 0 {
		return ""
	}
	var parts []string
	i := 0
	for i < len(tools) {
		tool := tools[i]
		count := 1
		for i+count < len(tools) && tools[i+count] == tool {
			count++
		}
		if count > 1 {
			parts = append(parts, fmt.Sprintf("%s{%d+}", tool, count))
		} else {
			parts = append(parts, tool)
		}
		i += count
	}
	return strings.Join(parts, " → ")
}

// patternKey returns a normalized abstract pattern for clustering purposes.
// All consecutive runs of 2+ collapse to {2+} so Read{3} and Read{5} cluster together.
func patternKey(tools []string) string {
	if len(tools) == 0 {
		return ""
	}
	var parts []string
	i := 0
	for i < len(tools) {
		tool := tools[i]
		count := 1
		for i+count < len(tools) && tools[i+count] == tool {
			count++
		}
		if count > 1 {
			parts = append(parts, tool+"{2+}")
		} else {
			parts = append(parts, tool)
		}
		i += count
	}
	return strings.Join(parts, " → ")
}

type patternStats struct {
	Pattern   string  `json:"pattern"`
	Count     int     `json:"count"`
	AvgLength float64 `json:"avg_length"`
	Sessions  int     `json:"sessions"`
}

func outputPatterns(w io.Writer, turns []store.TurnRow) error {
	// Cluster turns by abstract pattern.
	type cluster struct {
		totalLen int
		count    int
		sessions map[string]bool
	}
	clusters := make(map[string]*cluster)
	for _, t := range turns {
		key := patternKey(t.Tools)
		c, ok := clusters[key]
		if !ok {
			c = &cluster{sessions: make(map[string]bool)}
			clusters[key] = c
		}
		c.count++
		c.totalLen += t.Length
		c.sessions[t.SessionID] = true
	}

	// Sort by count descending.
	stats := make([]patternStats, 0, len(clusters))
	for pattern, c := range clusters {
		stats = append(stats, patternStats{
			Pattern:   pattern,
			Count:     c.count,
			AvgLength: float64(c.totalLen) / float64(c.count),
			Sessions:  len(c.sessions),
		})
	}
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Count > stats[j].Count
	})

	if jsonOutput {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(stats)
	}

	tbl := NewTable(w, "PATTERN", "COUNT", "AVG_LENGTH", "SESSIONS")
	for _, s := range stats {
		tbl.Row(
			s.Pattern,
			fmt.Sprintf("%d", s.Count),
			fmt.Sprintf("%.1f", s.AvgLength),
			fmt.Sprintf("%d", s.Sessions),
		)
	}
	tbl.Flush()
	return nil
}

func outputPatternDrilldown(w io.Writer, turns []store.TurnRow, pattern string) error {
	// Filter turns matching the given abstract pattern.
	var matching []store.TurnRow
	for _, t := range turns {
		if patternKey(t.Tools) == pattern {
			matching = append(matching, t)
		}
	}

	if jsonOutput {
		return writeTurnsJSON(w, matching)
	}
	writeTurnsTable(w, matching)
	return nil
}
