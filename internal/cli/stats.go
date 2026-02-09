package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

var showInvocations bool

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show summary statistics about recorded desires",
	Long: `Display a summary of desire-path data including total desires,
unique tool names, top sources, most common desires, date range,
and recent activity counts.

Use --invocations to display invocation statistics instead: total
invocations, unique tools, top sources, top tools, and time windows.`,
	Example: `  dp stats
  dp stats --invocations
  dp stats --invocations --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		if showInvocations {
			ist, err := s.InvocationStats(context.Background())
			if err != nil {
				return fmt.Errorf("get invocation stats: %w", err)
			}
			if jsonOutput {
				return printInvocationStatsJSON(ist)
			}
			printInvocationStatsText(ist)
			return nil
		}

		st, err := s.Stats(context.Background())
		if err != nil {
			return fmt.Errorf("get stats: %w", err)
		}

		if jsonOutput {
			return printStatsJSON(st)
		}
		printStatsText(st)
		return nil
	},
}

func init() {
	statsCmd.Flags().BoolVar(&showInvocations, "invocations", false, "show invocation statistics instead of desire statistics")
	rootCmd.AddCommand(statsCmd)
}

func printStatsJSON(st store.Stats) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(st)
}

func printStatsText(st store.Stats) {
	color := isTTY(os.Stdout)

	fmt.Printf("Total desires:      %d\n", st.TotalDesires)
	fmt.Printf("Unique tool names:  %d\n", st.UniquePaths)

	if st.TotalDesires == 0 {
		return
	}

	fmt.Println()

	// Date range.
	fmt.Printf("Date range:         %s to %s\n",
		st.Earliest.Format("2006-01-02"), st.Latest.Format("2006-01-02"))

	// Recent activity.
	fmt.Println()
	fmt.Printf("Last 24h:           %d\n", st.Last24h)
	fmt.Printf("Last 7d:            %d\n", st.Last7d)
	fmt.Printf("Last 30d:           %d\n", st.Last30d)

	// Top sources.
	if len(st.TopSources) > 0 {
		fmt.Println()
		fmt.Println(bold("Top sources:", color))

		// Sort sources by count descending for consistent output.
		type kv struct {
			key string
			val int
		}
		var sorted []kv
		for k, v := range st.TopSources {
			sorted = append(sorted, kv{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].val != sorted[j].val {
				return sorted[i].val > sorted[j].val
			}
			return sorted[i].key < sorted[j].key
		})
		for _, s := range sorted {
			fmt.Printf("  %-20s %d\n", s.key, s.val)
		}
	}

	// Top desires.
	if len(st.TopDesires) > 0 {
		fmt.Println()
		fmt.Println(bold("Top desires:", color))
		for _, d := range st.TopDesires {
			fmt.Printf("  %-20s %d\n", d.Name, d.Count)
		}
	}
}

func printInvocationStatsJSON(ist store.InvocationStatsResult) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(ist)
}

func printInvocationStatsText(ist store.InvocationStatsResult) {
	color := isTTY(os.Stdout)

	fmt.Printf("Total invocations:  %d\n", ist.Total)
	fmt.Printf("Unique tools:       %d\n", ist.UniqueTools)

	if ist.Total == 0 {
		return
	}

	fmt.Println()

	// Date range.
	fmt.Printf("Date range:         %s to %s\n",
		ist.Earliest.Format("2006-01-02"), ist.Latest.Format("2006-01-02"))

	// Recent activity.
	fmt.Println()
	fmt.Printf("Last 24h:           %d\n", ist.Last24h)
	fmt.Printf("Last 7d:            %d\n", ist.Last7d)
	fmt.Printf("Last 30d:           %d\n", ist.Last30d)

	// Top sources.
	if len(ist.TopSources) > 0 {
		fmt.Println()
		fmt.Println(bold("Top sources:", color))
		for _, s := range ist.TopSources {
			fmt.Printf("  %-20s %d\n", s.Name, s.Count)
		}
	}

	// Top tools.
	if len(ist.TopTools) > 0 {
		fmt.Println()
		fmt.Println(bold("Top tools:", color))
		for _, t := range ist.TopTools {
			fmt.Printf("  %-20s %d\n", t.Name, t.Count)
		}
	}
}
