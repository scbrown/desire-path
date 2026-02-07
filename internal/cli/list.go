package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

var (
	listSince  string
	listSource string
	listTool   string
	listLimit  int
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent desires",
	Long: `List displays a table of recorded desires (failed AI tool calls).
Results are ordered by timestamp, newest first.`,
	Example: `  dp list
  dp list --since 7d
  dp list --source claude-code --limit 20
  dp list --tool read_file --since 24h
  dp list --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer s.Close()

		opts := store.ListOpts{
			Source:   listSource,
			ToolName: listTool,
			Limit:    listLimit,
		}

		if listSince != "" {
			d, err := parseDuration(listSince)
			if err != nil {
				return fmt.Errorf("invalid --since value %q: %w", listSince, err)
			}
			opts.Since = time.Now().Add(-d)
		}

		desires, err := s.ListDesires(context.Background(), opts)
		if err != nil {
			return fmt.Errorf("list desires: %w", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(desires)
		}

		if len(desires) == 0 {
			fmt.Println("No desires found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "TIMESTAMP\tSOURCE\tTOOL\tERROR")
		for _, d := range desires {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				d.Timestamp.Format(time.DateTime),
				d.Source,
				d.ToolName,
				truncate(d.Error, 50),
			)
		}
		return w.Flush()
	},
}

func init() {
	listCmd.Flags().StringVar(&listSince, "since", "", "show desires within this duration (e.g., 30m, 24h, 7d)")
	listCmd.Flags().StringVar(&listSource, "source", "", "filter by source")
	listCmd.Flags().StringVar(&listTool, "tool", "", "filter by tool name")
	listCmd.Flags().IntVar(&listLimit, "limit", 50, "maximum number of results")
	rootCmd.AddCommand(listCmd)
}

// parseDuration parses a duration string that supports d (days), h (hours), m (minutes), s (seconds).
func parseDuration(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	// Handle "d" suffix for days, which time.ParseDuration doesn't support.
	if strings.HasSuffix(s, "d") {
		numStr := s[:len(s)-1]
		days, err := strconv.Atoi(numStr)
		if err != nil {
			return 0, fmt.Errorf("invalid day count %q", numStr)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}

// truncate shortens a string to max characters, appending "..." if truncated.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
