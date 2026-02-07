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

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show summary statistics about recorded desires",
	Long: `Display a summary of desire-path data including total desires,
unique tool names, top sources, most common desires, date range,
and recent activity counts.`,
	Example: `  dp stats
  dp stats --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer s.Close()

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
	rootCmd.AddCommand(statsCmd)
}

func printStatsJSON(st store.Stats) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(st)
}

func printStatsText(st store.Stats) {
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
		fmt.Println("Top sources:")

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
		fmt.Println("Top desires:")
		for _, d := range st.TopDesires {
			fmt.Printf("  %-20s %d\n", d.Name, d.Count)
		}
	}
}
