package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

var (
	inspectSince string
	inspectTopN  int
)

var inspectCmd = &cobra.Command{
	Use:   "inspect <pattern>",
	Short: "Show detailed view of a specific desire path",
	Long: `Inspect displays a detailed analysis of desires matching a tool name pattern.

Shows all matching desires aggregated with:
  - Summary (total count, first/last seen, alias if configured)
  - Frequency over time (text histogram by day)
  - Most common tool_input values
  - Most common error messages

The pattern argument is an exact tool name by default. Use % as a wildcard
for broader matching (e.g., "read%" matches read_file, read_dir, etc.).`,
	Example: `  dp inspect read_file
  dp inspect "read%"
  dp inspect Bash --since 7d
  dp inspect read_file --top 10
  dp inspect read_file --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		opts := store.InspectOpts{
			Pattern: args[0],
			TopN:    inspectTopN,
		}

		if inspectSince != "" {
			d, err := parseDuration(inspectSince)
			if err != nil {
				return fmt.Errorf("invalid --since value %q: %w", inspectSince, err)
			}
			opts.Since = time.Now().Add(-d)
		}

		result, err := s.InspectPath(context.Background(), opts)
		if err != nil {
			return fmt.Errorf("inspect path: %w", err)
		}

		w := cmd.OutOrStdout()

		if jsonOutput {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}

		writeInspectText(w, result)
		return nil
	},
}

func init() {
	inspectCmd.Flags().StringVar(&inspectSince, "since", "", "only include desires within this duration (e.g., 30m, 24h, 7d)")
	inspectCmd.Flags().IntVar(&inspectTopN, "top", 5, "number of top inputs/errors to display")
	rootCmd.AddCommand(inspectCmd)
}

func writeInspectText(w io.Writer, r *store.InspectResult) {
	color := isTTY(w)
	width := defaultTermWidth
	if color {
		width = getTermWidth()
	}

	fmt.Fprintf(w, "Pattern:    %s\n", r.Pattern)
	fmt.Fprintf(w, "Total:      %d\n", r.Total)

	if r.Total == 0 {
		fmt.Fprintln(w, "\nNo desires found for this pattern.")
		return
	}

	fmt.Fprintf(w, "First seen: %s\n", r.FirstSeen.UTC().Format(time.RFC3339))
	fmt.Fprintf(w, "Last seen:  %s\n", r.LastSeen.UTC().Format(time.RFC3339))
	if r.AliasTo != "" {
		fmt.Fprintf(w, "Alias:      %s\n", r.AliasTo)
	}

	// Frequency histogram.
	if len(r.Histogram) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, bold("Frequency:", color))

		maxCount := 0
		for _, h := range r.Histogram {
			if h.Count > maxCount {
				maxCount = h.Count
			}
		}

		maxBarWidth := width - 20
		if maxBarWidth < 10 {
			maxBarWidth = 10
		}
		if maxBarWidth > 40 {
			maxBarWidth = 40
		}
		for _, h := range r.Histogram {
			barLen := h.Count * maxBarWidth / maxCount
			if barLen == 0 && h.Count > 0 {
				barLen = 1
			}
			bar := strings.Repeat("#", barLen)
			fmt.Fprintf(w, "  %s  %s %d\n", h.Date, bar, h.Count)
		}
	}

	// Top tool inputs.
	if len(r.TopInputs) > 0 {
		maxName := width - 10
		if maxName < 30 {
			maxName = 30
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, bold("Top inputs:", color))
		for _, inp := range r.TopInputs {
			fmt.Fprintf(w, "  %4d  %s\n", inp.Count, truncate(inp.Name, maxName))
		}
	}

	// Top errors.
	if len(r.TopErrors) > 0 {
		maxName := width - 10
		if maxName < 30 {
			maxName = 30
		}
		fmt.Fprintln(w)
		fmt.Fprintln(w, bold("Top errors:", color))
		for _, e := range r.TopErrors {
			fmt.Fprintf(w, "  %4d  %s\n", e.Count, truncate(e.Name, maxName))
		}
	}
}
