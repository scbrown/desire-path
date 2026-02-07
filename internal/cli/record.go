package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/scbrown/desire-path/internal/record"
	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

var recordSource string

var recordCmd = &cobra.Command{
	Use:   "record",
	Short: "Record a failed tool call from stdin",
	Long: `Record reads a JSON object from stdin describing a failed AI tool call
and stores it in the desire-path database.

The JSON must contain at least a "tool_name" field. All other fields are optional.
Unknown fields are collected into metadata.`,
	Example: `  echo '{"tool_name":"read_file","error":"unknown tool"}' | dp record
  echo '{"tool_name":"run_tests","error":"not found"}' | dp record --source claude-code`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
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
