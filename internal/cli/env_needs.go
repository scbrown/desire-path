package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/scbrown/desire-path/internal/analyze"
	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

var envNeedsSince string

// envNeedsCmd surfaces missing-tool desires as actionable install recommendations.
var envNeedsCmd = &cobra.Command{
	Use:   "env-needs",
	Short: "Show missing tools that could be installed",
	Long: `Env-needs surfaces environment dependency gaps discovered from repeated
"command not found" errors. Instead of showing generic "Bash failed N times",
it identifies the specific missing commands and suggests installation.

These are desires categorized as "env-need" — failed Bash tool calls where the
error indicates a missing command (exit code 127, "command not found", etc.).`,
	Example: `  dp env-needs
  dp env-needs --since 7d
  dp env-needs --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		opts := store.ListOpts{
			Category: model.CategoryEnvNeed,
			Limit:    0, // all
		}

		if envNeedsSince != "" {
			d, err := parseDuration(envNeedsSince)
			if err != nil {
				return fmt.Errorf("invalid --since value %q: %w", envNeedsSince, err)
			}
			opts.Since = time.Now().Add(-d)
		}

		desires, err := s.ListDesires(context.Background(), opts)
		if err != nil {
			return fmt.Errorf("list env-need desires: %w", err)
		}

		// Aggregate by missing command.
		needs := aggregateEnvNeeds(desires)

		w := cmd.OutOrStdout()
		if jsonOutput {
			return writeEnvNeedsJSON(w, needs)
		}
		writeEnvNeedsTable(w, needs)
		return nil
	},
}

func init() {
	envNeedsCmd.Flags().StringVar(&envNeedsSince, "since", "", "show needs within this duration (e.g., 30m, 24h, 7d)")
	rootCmd.AddCommand(envNeedsCmd)
}

// EnvNeed represents an aggregated missing-command need.
type EnvNeed struct {
	Command   string `json:"command"`
	Count     int    `json:"count"`
	LastError string `json:"last_error"`
}

// aggregateEnvNeeds groups env-need desires by missing command name.
func aggregateEnvNeeds(desires []model.Desire) []EnvNeed {
	type entry struct {
		count     int
		lastError string
	}
	byCmd := make(map[string]*entry)
	var order []string

	for _, d := range desires {
		cmd := analyze.EnvNeedCommand(d.Error, d.ToolInput)
		if cmd == "" {
			cmd = "(unknown)"
		}
		e, ok := byCmd[cmd]
		if !ok {
			e = &entry{}
			byCmd[cmd] = e
			order = append(order, cmd)
		}
		e.count++
		if e.lastError == "" {
			e.lastError = d.Error
		}
	}

	// Sort by count descending (stable within insertion order for ties).
	needs := make([]EnvNeed, 0, len(order))
	for _, cmd := range order {
		e := byCmd[cmd]
		needs = append(needs, EnvNeed{
			Command:   cmd,
			Count:     e.count,
			LastError: e.lastError,
		})
	}
	sortEnvNeeds(needs)
	return needs
}

// sortEnvNeeds sorts by count descending using insertion sort.
func sortEnvNeeds(needs []EnvNeed) {
	for i := 1; i < len(needs); i++ {
		key := needs[i]
		j := i - 1
		for j >= 0 && needs[j].Count < key.Count {
			needs[j+1] = needs[j]
			j--
		}
		needs[j+1] = key
	}
}

func writeEnvNeedsJSON(w io.Writer, needs []EnvNeed) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if needs == nil {
		needs = []EnvNeed{}
	}
	return enc.Encode(needs)
}

func writeEnvNeedsTable(w io.Writer, needs []EnvNeed) {
	if len(needs) == 0 {
		fmt.Fprintln(os.Stderr, "No missing tools detected.")
		return
	}
	fmt.Fprintf(w, "Missing tools (%d):\n\n", len(needs))
	tbl := NewTable(w, "COMMAND", "FAILURES", "ACTION")
	for _, n := range needs {
		action := fmt.Sprintf("Install %s", n.Command)
		tbl.Row(n.Command, fmt.Sprintf("%d", n.Count), action)
	}
	tbl.Flush()
}
