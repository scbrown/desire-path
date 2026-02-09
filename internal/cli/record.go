package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/scbrown/desire-path/internal/record"
	"github.com/spf13/cobra"
)

var recordSource string

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record a failed tool call from stdin (deprecated: use dp ingest)",
	Long: `Deprecated: use "dp ingest --source <name>" instead.

Record reads a JSON object from stdin describing a failed AI tool call
and stores it in the desire-path database.

When --source is specified, record delegates to the same ingest pipeline
used by "dp ingest", creating an invocation record (and a desire when the
call is an error). Without --source, the legacy behavior is preserved:
the JSON must contain at least a "tool_name" field, and a desire is recorded
directly.`,
	Example: `  # Preferred: use dp ingest directly
  echo '{"tool_name":"Read","session_id":"s1","cwd":"/tmp"}' | dp ingest --source claude-code

  # Legacy (still works):
  echo '{"tool_name":"read_file","error":"unknown tool"}' | dp record
  echo '{"tool_name":"run_tests","error":"not found"}' | dp record --source claude-code`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// When --source is provided, delegate to the ingest pipeline.
		if recordSource != "" {
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
		}

		// Legacy path: no --source, generic JSON → desire only.
		s, err := openStore()
		if err != nil {
			return fmt.Errorf("open store: %w", err)
		}
		defer s.Close()

		d, err := record.Record(context.Background(), s, os.Stdin, recordSource)
		if err != nil {
			return err
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(d)
		}
		fmt.Fprintf(os.Stderr, "Recorded desire: %s (tool: %s)\n", d.ID, d.ToolName)
		return nil
	},
}

func init() {
	recordCmd.Flags().StringVar(&recordSource, "source", "", "source identifier (e.g., claude-code)")
	rootCmd.AddCommand(recordCmd)
}
