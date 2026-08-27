package cli

import (
	"context"
	"io"
	"os"
	"os/exec"
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

var signpostPrefetchCmd = &cobra.Command{Use: "signpost-prefetch", Hidden: true, RunE: runSignpostPrefetch}
var signpostFetchCmd = &cobra.Command{Use: "signpost-fetch", Hidden: true, RunE: runSignpostFetch}
var fetchID, fetchQuery string

func init() {
	signpostFetchCmd.Flags().StringVar(&fetchID, "id", "", "tool-use join key")
	signpostFetchCmd.Flags().StringVar(&fetchQuery, "query", "", "semantic query")
	rootCmd.AddCommand(signpostCmd, signpostPrefetchCmd, signpostFetchCmd)
}

func signpostCacheDir() string {
	return env("DP_SIGNPOST_CACHE_DIR", os.TempDir()+"/desire-path-signpost")
}

func runSignpostPrefetch(_ *cobra.Command, _ []string) error {
	if os.Getenv("DP_SIGNPOST_PREFETCH") == "0" {
		return nil
	}
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil
	}
	_, query, ok := signpost.PrefetchRequest(raw)
	if !ok {
		return nil
	}
	id := signpost.CacheKey(os.Getenv("DP_SIGNPOST_REPO"), query)
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	child := exec.Command(exe, "signpost-fetch", "--id", id, "--query", query)
	child.Stdin, child.Stdout, child.Stderr = nil, nil, nil
	if child.Start() == nil {
		_ = child.Process.Release()
	}
	return nil
}

func runSignpostFetch(cmd *cobra.Command, _ []string) error {
	if fetchID == "" || fetchQuery == "" {
		return nil
	}
	timeout := time.Duration(envInt("DP_SIGNPOST_PREFETCH_TIMEOUT_MS", 5000)) * time.Millisecond
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()
	count, err := signpost.HTTPSearcher(env("DP_SIGNPOST_BOBBIN_URL", "http://localhost:3000/search"), os.Getenv("DP_SIGNPOST_REPO"), timeout)(ctx, fetchQuery)
	if err == nil {
		_ = signpost.WriteWarmResult(signpostCacheDir(), fetchID, count)
	}
	return nil
}

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
		TaskID: os.Getenv("DP_SIGNPOST_TASK_ID"), Model: os.Getenv("DP_SIGNPOST_MODEL_FAMILY"), Repo: os.Getenv("DP_SIGNPOST_REPO"), CacheDir: signpostCacheDir()}
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()
	out, event, err := signpost.Process(ctx, raw, cfg, signpost.HTTPSearcher(cfg.BobbinURL, cfg.Repo, timeout))
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
