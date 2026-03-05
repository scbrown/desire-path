package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

var (
	recoverySince string
	recoveryLimit int
	recoveryStats bool
)

var recoveriesCmd = &cobra.Command{
	Use:   "recoveries",
	Short: "Show recovery events (previously-failing tools that succeeded)",
	Long: `List recovery events where a tool that was previously failing started
succeeding again. Use --stats for aggregated counts per tool.`,
	Example: `  dp recoveries
  dp recoveries --since 7d
  dp recoveries --stats
  dp recoveries --stats --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := openStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()
		ctx := context.Background()

		if recoveryStats {
			stats, err := s.RecoveryStats(ctx)
			if err != nil {
				return fmt.Errorf("recovery stats: %w", err)
			}
			if jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(stats)
			}
			if len(stats) == 0 {
				fmt.Println("No recoveries recorded yet.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "TOOL\tCOUNT\tLAST RECOVERY")
			for _, st := range stats {
				fmt.Fprintf(w, "%s\t%d\t%s\n", st.ToolName, st.Count, st.LastRecovery.Format(time.RFC3339))
			}
			return w.Flush()
		}

		var since time.Time
		if recoverySince != "" {
			d, err := parseDuration(recoverySince)
			if err != nil {
				return fmt.Errorf("invalid --since value %q: %w", recoverySince, err)
			}
			since = time.Now().Add(-d)
		}

		recoveries, err := s.ListRecoveries(ctx, since, recoveryLimit)
		if err != nil {
			return fmt.Errorf("list recoveries: %w", err)
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(recoveries)
		}

		if len(recoveries) == 0 {
			fmt.Println("No recoveries found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "TOOL\tDESIRE ID\tTIMESTAMP")
		for _, r := range recoveries {
			fmt.Fprintf(w, "%s\t%s\t%s\n", r.ToolName, r.DesireID, r.Timestamp.Format(time.RFC3339))
		}
		return w.Flush()
	},
}

func init() {
	recoveriesCmd.Flags().StringVar(&recoverySince, "since", "", "show recoveries since duration (e.g. 7d, 24h)")
	recoveriesCmd.Flags().IntVar(&recoveryLimit, "limit", 50, "maximum results")
	recoveriesCmd.Flags().BoolVar(&recoveryStats, "stats", false, "show aggregated recovery counts per tool")
	rootCmd.AddCommand(recoveriesCmd)
}
