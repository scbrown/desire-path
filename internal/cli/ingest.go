package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/scbrown/desire-path/internal/config"
	"github.com/scbrown/desire-path/internal/ingest"
	"github.com/scbrown/desire-path/internal/metrics"
	"github.com/scbrown/desire-path/internal/model"
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

		inv, err := doIngest(ingestSource)
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
		fmt.Fprintf(os.Stderr, "Ingested invocation: %s (source: %s, tool: %s)\n", inv.ID, inv.Source, inv.ToolName)
		return nil
	},
}

// doIngest runs the ingest pipeline: read stdin, extract fields via source
// plugin, check track_tools allowlist, and persist the invocation. Returns
// nil invocation (and nil error) when the tool is filtered by the allowlist.
func doIngest(sourceName string) (*model.Invocation, error) {
	started := time.Now()
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("reading stdin: %w", err)
	}

	src := source.Get(sourceName)
	if src == nil {
		return nil, fmt.Errorf("unknown source: %q", sourceName)
	}
	fields, err := src.Extract(raw)
	if err != nil {
		return nil, fmt.Errorf("extracting fields: %w", err)
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
			return nil, nil
		}
	}

	s, err := openStore()
	if err != nil {
		return nil, fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	inv, err := ingest.IngestFields(context.Background(), s, fields, sourceName)
	if err != nil {
		reportMetrics(s, sourceName, started, 1)
		return nil, err
	}

	// FUNCTIONAL COUNTERS (aegis-lu5502). Pushed, because `dp ingest` runs once
	// per tool call and exits — there is nothing alive to scrape. What is pushed
	// is the STORE's own totals, not a per-run count: a Pushgateway replaces a
	// job's samples, so "I did 1 ingest" would sit at a flat 1 forever however
	// much work happened. Store totals move with the work, and they are the
	// numbers `dp stats` already reports. Never fatal — see internal/metrics.
	reportMetrics(s, sourceName, started, 0)

	return &inv, nil
}

// reportMetrics pushes the store's totals and this run's liveness. It swallows
// everything: an ingest must not fail because a metrics gateway is unreachable,
// unauthenticated or unconfigured.
func reportMetrics(s store.Store, sourceName string, started time.Time, status int) {
	var samples []metrics.Sample
	if status == 0 {
		st, err := s.Stats(context.Background())
		if err != nil {
			fmt.Fprintf(os.Stderr, "metrics: store totals unavailable (%v) — producer group still pushed\n", err)
			status = 0 // the INGEST succeeded; only the reading failed
		} else {
			// NO source label: store.Stats is an unfiltered COUNT over the whole
			// `desires` table, so these are GLOBAL totals. Labelling a global
			// number with whichever source happened to trigger the push made it
			// read as per-source AND destroyed the previous source's series on
			// every push (measured, aegis-lu5502). Per-source counts already
			// exist as st.TopSources if we ever want them.
			samples = []metrics.Sample{
				{Name: "desire_path_desires_total", Value: float64(st.TotalDesires)},
				{Name: "desire_path_unique_paths", Value: float64(st.UniquePaths)},
				{Name: "desire_path_desires_last_24h", Value: float64(st.Last24h)},
				{Name: "desire_path_desires_last_7d", Value: float64(st.Last7d)},
			}
		}
	}
	metrics.Report(os.Stderr, sourceName, samples, started, status)
}

func init() {
	ingestCmd.Flags().StringVar(&ingestSource, "source", "", "source plugin name (required)")
	rootCmd.AddCommand(ingestCmd)
}
