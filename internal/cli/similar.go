package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/scbrown/desire-path/internal/analyze"
	"github.com/spf13/cobra"
)

// defaultKnownTools lists Claude Code built-in tools.
var defaultKnownTools = []string{
	"Read", "Write", "Edit", "Bash", "Glob", "Grep",
	"Task", "WebFetch", "WebSearch", "NotebookEdit",
}

var (
	similarKnown     string
	similarThreshold float64
	similarTopN      int
)

// similarCmd finds known tools similar to a given tool name.
var similarCmd = &cobra.Command{
	Use:   "similar <tool-name>",
	Short: "Find known tools similar to a tool name",
	Long: `Similar finds known tools similar to the given name using string similarity.
It checks configured aliases for an exact mapping first, then ranks known tools
by similarity score. Known tools default to Claude Code built-ins but can be
overridden with the --known flag.`,
	Example: `  dp similar read_file
  dp similar WriteFile --known "Read,Write,Edit,Bash"
  dp similar edit_file --threshold 0.3 --top 3
  dp similar read_file --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolName := args[0]
		w := cmd.OutOrStdout()

		s, err := openStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		// Check for existing alias first.
		aliases, err := s.GetAliases(context.Background())
		if err != nil {
			return fmt.Errorf("get aliases: %w", err)
		}
		for _, a := range aliases {
			if strings.EqualFold(a.From, toolName) {
				if jsonOutput {
					return writeSimilarAliasJSON(w, toolName, a.To)
				}
				fmt.Fprintf(w, "Alias: %q → %q\n", toolName, a.To)
				return nil
			}
		}

		// Build known tools list.
		known := defaultKnownTools
		if similarKnown != "" {
			known = strings.Split(similarKnown, ",")
			for i := range known {
				known[i] = strings.TrimSpace(known[i])
			}
		}

		threshold := similarThreshold
		if threshold == 0 {
			threshold = analyze.DefaultThreshold
		}
		suggestions := analyze.SuggestN(toolName, known, similarTopN, threshold)

		if jsonOutput {
			return writeSimilarJSON(w, toolName, suggestions)
		}
		writeSimilarTable(w, toolName, suggestions)
		return nil
	},
}

func init() {
	similarCmd.Flags().StringVar(&similarKnown, "known", "", "comma-separated list of known tool names")
	similarCmd.Flags().Float64Var(&similarThreshold, "threshold", 0, "minimum similarity score (default 0.5)")
	similarCmd.Flags().IntVar(&similarTopN, "top", 5, "maximum number of suggestions")
	rootCmd.AddCommand(similarCmd)
}

// similarOutput is the JSON structure for similar results.
type similarOutput struct {
	Query       string               `json:"query"`
	Alias       string               `json:"alias,omitempty"`
	Suggestions []analyze.Suggestion `json:"suggestions,omitempty"`
}

// writeSimilarAliasJSON writes an alias match as JSON.
func writeSimilarAliasJSON(w io.Writer, query, alias string) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(similarOutput{Query: query, Alias: alias})
}

// writeSimilarJSON writes suggestions as JSON.
func writeSimilarJSON(w io.Writer, query string, suggestions []analyze.Suggestion) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(similarOutput{Query: query, Suggestions: suggestions})
}

// writeSimilarTable writes suggestions as an aligned text table.
func writeSimilarTable(w io.Writer, query string, suggestions []analyze.Suggestion) {
	if len(suggestions) == 0 {
		fmt.Fprintf(w, "No suggestions found for %q\n", query)
		return
	}
	tbl := NewTable(w, "RANK", "TOOL", "SCORE")
	for i, s := range suggestions {
		tbl.Row(fmt.Sprintf("%d", i+1), s.Name, fmt.Sprintf("%.2f", s.Score))
	}
	tbl.Flush()
}
