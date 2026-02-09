package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/scbrown/desire-path/internal/config"
	"github.com/scbrown/desire-path/internal/ingest"
	"github.com/scbrown/desire-path/internal/source"
	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

var ingestSource string

var ingestCmd = &cobra.Command{
	Use:   "ingest",
	Short: "Ingest tool call data from a source plugin",
	Long: `Ingest reads raw tool call data from stdin, extracts structured fields
using the specified source plugin, and records the resulting invocation
to the database.

Each source plugin knows how to parse a specific AI tool's output format
(e.g., Claude Code, Cursor). The --source flag selects which plugin to use.

Use "dp ingest --source <name>" with data piped to stdin.`,
	Example: `  # Ingest a Claude Code tool call
  cat tool-call.json | dp ingest --source claude-code

  # Ingest with JSON output
  echo '{"tool":"Read","input":{"path":"foo.txt"}}' | dp ingest --source claude-code --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if ingestSource == "" {
			names := source.Names()
			if len(names) == 0 {
				return fmt.Errorf("--source flag is required (no sources registered)")
			}
			return fmt.Errorf("--source flag is required (available: %s)", strings.Join(names, ", "))
		}

		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("reading stdin: %w", err)
		}

		// Extract fields first so we can check the tool name against the allowlist.
		src := source.Get(ingestSource)
		if src == nil {
			return fmt.Errorf("unknown source: %q", ingestSource)
		}
		fields, err := src.Extract(raw)
		if err != nil {
			return fmt.Errorf("extracting fields: %w", err)
		}

		// Check track_tools allowlist: if non-empty and tool not listed, skip silently.
		cfg, cfgErr := config.LoadFrom(configPath)
		if cfgErr == nil && len(cfg.TrackTools) > 0 {
			allowed := false
			for _, t := range cfg.TrackTools {
				if t == fields.ToolName {
					allowed = true
					break
				}
			}
			if !allowed {
				return nil
			}
		}

		s, err := store.New(dbPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer s.Close()

		inv, err := ingest.IngestFields(context.Background(), s, fields, ingestSource)
		if err != nil {
			return err
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(inv)
		}
		fmt.Fprintf(os.Stderr, "Ingested invocation: %s (source: %s, tool: %s)\n", inv.ID, inv.Source, inv.ToolName)
		return nil
	},
}

func init() {
	ingestCmd.Flags().StringVar(&ingestSource, "source", "", "source plugin name (required)")
	rootCmd.AddCommand(ingestCmd)
}
