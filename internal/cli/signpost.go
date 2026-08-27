package cli

import (
	"context"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/scbrown/desire-path/internal/signpost"
	"github.com/spf13/cobra"
)

var signpostCmd = &cobra.Command{
	Use:     "signpost",
	Short:   "Offer semantic search after weak literal-search outcomes (hook internal)",
	Long:    "Reads a Claude Code PostToolUse payload from stdin. Bash-mediated grep and rg calls are evaluated after completion; eligible results receive a Bobbin command as disjoint hook context. Failures and timeouts stay silent.",
	Example: `  echo '{"tool_name":"Bash","tool_input":{"command":"rg retry ."},"tool_response":""}' | dp signpost`,
	RunE:    runSignpost,
}

func init() { rootCmd.AddCommand(signpostCmd) }

func runSignpost(cmd *cobra.Command, _ []string) error {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil
	}
	threshold := envInt("DP_SIGNPOST_THRESHOLD", 100)
	timeout := time.Duration(envInt("DP_SIGNPOST_TIMEOUT_MS", 150)) * time.Millisecond
	cfg := signpost.Config{Threshold: threshold, Timeout: timeout,
		BobbinURL: env("DP_SIGNPOST_BOBBIN_URL", "http://localhost:3000/search"),
		LogPath:   env("DP_SIGNPOST_LOG", ""), Condition: env("DP_SIGNPOST_CONDITION", "gated-signpost"),
		TaskID: os.Getenv("DP_SIGNPOST_TASK_ID"), Model: os.Getenv("DP_SIGNPOST_MODEL_FAMILY")}
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()
	out, event, err := signpost.Process(ctx, raw, cfg, signpost.HTTPSearcher(cfg.BobbinURL, timeout))
	if err != nil {
		return nil
	}
	if event.EventID != "" {
		_ = signpost.AppendEvent(cfg.LogPath, event)
	}
	if len(out) > 0 {
		_, _ = os.Stdout.Write(append(out, '\n'))
	}
	return nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func envInt(key string, fallback int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return v
}
