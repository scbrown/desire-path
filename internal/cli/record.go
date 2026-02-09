package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/scbrown/desire-path/internal/source"
	"github.com/spf13/cobra"
)

var recordSource string

var recordCmd = &cobra.Command{
	Use:        "record",
	Short:      "Record a tool call from stdin (alias for dp ingest)",
	Deprecated: `use "dp ingest --source <name>" instead`,
	Long: `Record is a legacy alias for "dp ingest". It delegates entirely to
the ingest pipeline, creating an invocation record (and a desire when
the call is an error via dual-write).

Use "dp ingest --source <name>" directly for new integrations.`,
	Example: `  # Preferred: use dp ingest directly
  echo '{"tool_name":"Read","session_id":"s1","cwd":"/tmp"}' | dp ingest --source claude-code

  # Legacy alias (delegates to ingest):
  echo '{"tool_name":"run_tests","error":"not found"}' | dp record --source claude-code`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if recordSource == "" {
			names := source.Names()
			if len(names) == 0 {
				return fmt.Errorf("--source flag is required (no sources registered)")
			}
			return fmt.Errorf("--source flag is required (available: %s)", strings.Join(names, ", "))
		}

		inv, err := doIngest(recordSource)
		if err != nil {
			return err
		}
		if inv == nil {
			return nil // filtered by allowlist
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(inv)
		}
		fmt.Fprintf(os.Stderr, "Recorded invocation: %s (source: %s, tool: %s)\n", inv.ID, inv.Source, inv.ToolName)
		return nil
	},
}

func init() {
	recordCmd.Flags().StringVar(&recordSource, "source", "", "source plugin name (required)")
	rootCmd.AddCommand(recordCmd)
}
