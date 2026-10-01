package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/scbrown/desire-path/internal/legend"
	"github.com/scbrown/desire-path/internal/signpost"
	"github.com/spf13/cobra"
)

var legendRefreshCmd = &cobra.Command{
	Use:   "legend-refresh",
	Short: "Rebuild the read-time entity legend gazetteer from Quipu",
	Long: `legend-refresh exports the labels of governed Quipu classes into the local
gazetteer the read-time entity legend matches against (aegis-9qgbff). Run it
from a timer; the hook itself never touches the network.

The legend is a second stage of 'dp signpost' and is OFF unless DP_LEGEND=1.
It annotates a Read (or a Bash cat/head/sed/br show) with the Quipu entities
the text names, inside what is left of the shared per-call context budget.

Environment:
  DP_LEGEND                    1 enables the stage (default off).
  DP_LEGEND_GAZETTEER          Gazetteer path (default <user cache>/desire-path/legend-gazetteer.json).
  DP_LEGEND_QUIPU_URL          Quipu /query endpoint for refresh (required).
  DP_LEGEND_NAMESPACE          Ontology namespace the class names live in (required).
  DP_LEGEND_MAX_BYTES          Combined budget shared with the signpost stage (default 600).
  DP_LEGEND_MAX_AGE_MIN        Refuse a gazetteer older than this (default 1440).
  DP_LEGEND_LOG                Decision log (default ~/.local/log/dp-legend-events.jsonl; "-" disables).
  DP_LEGEND_DICT               Word list for the precision guard (default /usr/share/dict/words).`,
	SilenceUsage: true,
	RunE:         runLegendRefresh,
}

func init() {
	rootCmd.AddCommand(legendRefreshCmd)
}

func legendGazetteerPath() string {
	if p := os.Getenv("DP_LEGEND_GAZETTEER"); p != "" {
		return p
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "desire-path", "legend-gazetteer.json")
}

func legendLogPath() string {
	if p, ok := os.LookupEnv("DP_LEGEND_LOG"); ok {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "log", "dp-legend-events.jsonl")
}

func runLegendRefresh(cmd *cobra.Command, _ []string) error {
	queryURL, namespace := os.Getenv("DP_LEGEND_QUIPU_URL"), os.Getenv("DP_LEGEND_NAMESPACE")
	if queryURL == "" || namespace == "" {
		return fmt.Errorf("legend-refresh needs DP_LEGEND_QUIPU_URL (the /query endpoint) and DP_LEGEND_NAMESPACE (the ontology namespace)")
	}
	dict := legend.LoadDict(env("DP_LEGEND_DICT", "/usr/share/dict/words"))
	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Minute)
	defer cancel()
	f, reports, err := legend.Refresh(ctx, queryURL, namespace, legend.DefaultClasses, dict)
	enc := json.NewEncoder(cmd.ErrOrStderr())
	for _, r := range reports {
		_ = enc.Encode(r)
	}
	if err != nil {
		return err
	}
	path := legendGazetteerPath()
	if err := legend.Write(path, f); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "wrote %d entries to %s\n", len(f.Entries), path)
	return nil
}

// withLegend runs the entity-legend stage after the signpost stage and folds
// its line into the same hook output. The legend spends only what the signpost
// stage left of the combined budget: when both fire, the signpost keeps its
// bytes because it answers the search the agent actually ran. Every failure
// leaves the signpost output exactly as it was.
func withLegend(raw, out []byte) []byte {
	if os.Getenv("DP_LEGEND") != "1" || !legend.IsRead(raw) {
		return out // most tool calls are not reads: no file I/O, no log line
	}
	hookEvent, prior := signpost.ContextOf(out)
	budget := envInt("DP_LEGEND_MAX_BYTES", legend.DefaultMaxBytes) - len(prior)
	cfg := legend.Config{
		GazetteerPath: legendGazetteerPath(),
		MaxAge:        time.Duration(envInt("DP_LEGEND_MAX_AGE_MIN", 1440)) * time.Minute,
		MaxEntities:   legend.DefaultMaxEntities,
		Budget:        budget,
		SeenDir:       filepath.Join(signpostCacheDir(), "legend-seen"),
	}
	var g *legend.Gazetteer
	loadStart := time.Now()
	if budget > 0 {
		g, _ = legend.Load(cfg.GazetteerPath, cfg.MaxAge, loadStart)
	}
	loadUS := time.Since(loadStart).Microseconds()
	text, event := legend.Process(raw, cfg, g)
	event.LoadUS = loadUS
	event.LatencyUS += loadUS
	_ = legend.AppendEvent(legendLogPath(), event)
	if text == "" {
		return out
	}
	if hookEvent == "" {
		hookEvent = "PostToolUse"
	}
	combined := text
	if prior != "" {
		combined = prior + "\n" + text
	}
	b, err := signpost.WithContext(hookEvent, combined)
	if err != nil {
		return out
	}
	return b
}
