package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

var (
	exportFormat string
	exportSince  string
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export raw desire data",
	Long: `Export dumps raw desire records in JSON (one per line) or CSV format.

Output is written to stdout, suitable for piping to jq, spreadsheets, or
other processing tools.`,
	Example: `  dp export
  dp export --format csv > desires.csv
  dp export --since 2024-01-01
  dp export --format json --since 2024-01-01T00:00:00Z | jq '.tool_name'`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer s.Close()

		var opts store.ListOpts
		if exportSince != "" {
			t, err := parseSince(exportSince)
			if err != nil {
				return fmt.Errorf("invalid --since value %q: %w", exportSince, err)
			}
			opts.Since = t
		}

		desires, err := s.ListDesires(context.Background(), opts)
		if err != nil {
			return fmt.Errorf("list desires: %w", err)
		}

		format := exportFormat
		if jsonOutput {
			format = "json"
		}

		switch format {
		case "json":
			return writeJSON(desires)
		case "csv":
			return writeCSV(desires)
		default:
			return fmt.Errorf("unsupported format %q (use json or csv)", format)
		}
	},
}

func init() {
	exportCmd.Flags().StringVar(&exportFormat, "format", "json", "output format: json or csv")
	exportCmd.Flags().StringVar(&exportSince, "since", "", "only export desires after this time (RFC3339 or YYYY-MM-DD)")
	rootCmd.AddCommand(exportCmd)
}

// parseSince parses a time string in RFC3339 or date-only format.
func parseSince(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("expected RFC3339 (e.g. 2024-01-01T00:00:00Z) or date (e.g. 2024-01-01)")
}

// writeJSON writes desires as one JSON object per line (JSONL).
func writeJSON(desires []model.Desire) error {
	enc := json.NewEncoder(os.Stdout)
	for _, d := range desires {
		if err := enc.Encode(d); err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
	}
	return nil
}

// writeCSV writes desires as CSV with a header row.
func writeCSV(desires []model.Desire) error {
	w := csv.NewWriter(os.Stdout)
	header := []string{"id", "tool_name", "error", "source", "session_id", "cwd", "timestamp", "tool_input", "metadata"}
	if err := w.Write(header); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	for _, d := range desires {
		row := []string{
			d.ID,
			d.ToolName,
			d.Error,
			d.Source,
			d.SessionID,
			d.CWD,
			d.Timestamp.Format(time.RFC3339),
			string(d.ToolInput),
			string(d.Metadata),
		}
		if err := w.Write(row); err != nil {
			return fmt.Errorf("write csv row: %w", err)
		}
	}
	w.Flush()
	return w.Error()
}
