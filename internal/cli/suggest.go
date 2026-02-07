package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/scbrown/desire-path/internal/analyze"
	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

// defaultKnownTools lists Claude Code built-in tools.
var defaultKnownTools = []string{
	"Read", "Write", "Edit", "Bash", "Glob", "Grep",
	"Task", "WebFetch", "WebSearch", "NotebookEdit",
}

var (
	suggestKnown     string
	suggestThreshold float64
	suggestTopN      int
)

// suggestCmd suggests known tool mappings for a given tool name.
var suggestCmd = &cobra.Command{
	Use:   "suggest <tool-name>",
	Short: "Suggest known tool mappings for a tool name",
	Long: `Suggest finds known tools similar to the given name using string similarity.
It checks configured aliases for an exact mapping first, then ranks known tools
by similarity score. Known tools default to Claude Code built-ins but can be
overridden with the --known flag.`,
	Example: `  dp suggest read_file
  dp suggest WriteFile --known "Read,Write,Edit,Bash"
  dp suggest edit_file --threshold 0.3 --top 3
  dp suggest read_file --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		toolName := args[0]
		w := cmd.OutOrStdout()

		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
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
					return writeSuggestAliasJSON(w, toolName, a.To)
				}
				fmt.Fprintf(w, "Alias: %q → %q\n", toolName, a.To)
				return nil
			}
		}

		// Build known tools list.
		known := defaultKnownTools
		if suggestKnown != "" {
			known = strings.Split(suggestKnown, ",")
			for i := range known {
				known[i] = strings.TrimSpace(known[i])
			}
		}

		threshold := suggestThreshold
		if threshold == 0 {
			threshold = analyze.DefaultThreshold
		}
		suggestions := analyze.SuggestN(toolName, known, suggestTopN, threshold)

		if jsonOutput {
			return writeSuggestJSON(w, toolName, suggestions)
		}
		writeSuggestTable(w, toolName, suggestions)
		return nil
	},
}

func init() {
	suggestCmd.Flags().StringVar(&suggestKnown, "known", "", "comma-separated list of known tool names")
	suggestCmd.Flags().Float64Var(&suggestThreshold, "threshold", 0, "minimum similarity score (default 0.5)")
	suggestCmd.Flags().IntVar(&suggestTopN, "top", 5, "maximum number of suggestions")
	rootCmd.AddCommand(suggestCmd)
}

// suggestOutput is the JSON structure for suggest results.
type suggestOutput struct {
	Query       string               `json:"query"`
	Alias       string               `json:"alias,omitempty"`
	Suggestions []analyze.Suggestion `json:"suggestions,omitempty"`
}

// writeSuggestAliasJSON writes an alias match as JSON.
func writeSuggestAliasJSON(w io.Writer, query, alias string) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(suggestOutput{Query: query, Alias: alias})
}

// writeSuggestJSON writes suggestions as JSON.
func writeSuggestJSON(w io.Writer, query string, suggestions []analyze.Suggestion) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(suggestOutput{Query: query, Suggestions: suggestions})
}

// writeSuggestTable writes suggestions as an aligned text table.
func writeSuggestTable(w io.Writer, query string, suggestions []analyze.Suggestion) {
	if len(suggestions) == 0 {
		fmt.Fprintf(w, "No suggestions found for %q\n", query)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "RANK\tTOOL\tSCORE")
	for i, s := range suggestions {
		fmt.Fprintf(tw, "%d\t%s\t%.2f\n", i+1, s.Name, s.Score)
	}
	tw.Flush()
}
