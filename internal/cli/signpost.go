package cli

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/scbrown/desire-path/internal/pave"
	"github.com/scbrown/desire-path/internal/signpost"
	"github.com/spf13/cobra"
)

var signpostCmd = &cobra.Command{
	Use:   "signpost",
	Short: "Offer semantic search after weak literal-search outcomes (hook internal)",
	Long: `Reads a Claude Code PostToolUse payload from stdin and discovers what the
command was ASKING, across four intent families: a literal search (grep, rg), a
file lookup (find -name), a history search (git log -S, --grep), and a symbol
lookup (a bare identifier that resolves in the index). Eligible results receive
the stack command that answers the question, as disjoint hook context. Failures
and timeouts stay silent, and the tool contract is never modified.

Environment:
  DP_SIGNPOST_CONDITION          always-signpost | gated-signpost |
                                 payload-signpost | payload-gated-signpost.
                                 Anything else never emits.
  DP_SIGNPOST_PAYLOAD=1          Inject the RESULT of the stack command instead
                                 of the command, on any emitting condition.
  DP_SIGNPOST_PAYLOAD_HITS       Locations injected in payload mode (default 3).
  DP_SIGNPOST_PAYLOAD_MAX_BYTES  Payload size cap (default 800).
  DP_SIGNPOST_THRESHOLD          Line count above which a result is high
                                 cardinality (default 100).
  DP_SIGNPOST_TIMEOUT_MS         Semantic lookup budget (default 150).
  DP_SIGNPOST_BOBBIN_URL         The /search route; /refs is derived from it.
  DP_SIGNPOST_REPO               Repository scope for the lookup.`,
	Example: `  echo '{"tool_name":"Bash","tool_input":{"command":"rg retry ."},"tool_response":""}' | dp signpost
  DP_SIGNPOST_CONDITION=payload-signpost dp signpost < payload.json`,
	RunE: runSignpost,
}

var signpostPrefetchCmd = &cobra.Command{Use: "signpost-prefetch", Hidden: true, RunE: runSignpostPrefetch}
var signpostFetchCmd = &cobra.Command{Use: "signpost-fetch", Hidden: true, RunE: runSignpostFetch}
var fetchID, fetchQuery, fetchFamily, fetchSymbol string

func init() {
	signpostFetchCmd.Flags().StringVar(&fetchID, "id", "", "tool-use join key")
	signpostFetchCmd.Flags().StringVar(&fetchQuery, "query", "", "semantic query")
	signpostFetchCmd.Flags().StringVar(&fetchFamily, "family", "", "discovered intent family")
	signpostFetchCmd.Flags().StringVar(&fetchSymbol, "symbol", "", "identifier to try resolving structurally")
	rootCmd.AddCommand(signpostCmd, signpostPrefetchCmd, signpostFetchCmd)
}

func signpostCacheDir() string {
	return env("DP_SIGNPOST_CACHE_DIR", os.TempDir()+"/desire-path-signpost")
}

func runSignpostPrefetch(_ *cobra.Command, _ []string) error {
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil
	}

	// DELIVER ANY PARKED CORRECTION FIRST, and unconditionally.
	//
	// This runs before the prefetch gate and outside it on purpose. A
	// correction is owed to the agent whatever its next command happens to be —
	// it is not about this call, it is about the failed one before it — so
	// gating it on the prefetch being enabled, or on this command looking like
	// a search, would drop it for most calls.
	//
	// PostToolUseFailure is where the correction is produced and where it
	// cannot be delivered: that event's additionalContext is discarded by the
	// harness (aegis-c2g1s5). PreToolUse injection is proven to reach the model
	// — every crew pane sees hook context here on each Bash call — so the
	// correction is carried across one turn and delivered where it lands.
	deliverParkedCorrection(raw)

	if os.Getenv("DP_SIGNPOST_PREFETCH") == "0" {
		return nil
	}
	_, intent, ok := signpost.PrefetchRequest(raw)
	if !ok {
		return nil
	}
	id := signpost.CacheKey(os.Getenv("DP_SIGNPOST_REPO"), intent.Query)
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	child := exec.Command(exe, "signpost-fetch", "--id", id, "--query", intent.Query,
		"--family", intent.Family, "--symbol", intent.SymbolCandidate)
	child.Stdin, child.Stdout, child.Stderr = nil, nil, nil
	if child.Start() == nil {
		_ = child.Process.Release()
	}
	return nil
}

// runSignpostFetch performs the speculative lookup out of band. It runs the
// SAME resolution Process would run, including the symbol promotion, and stores
// the family it settled on: the PostToolUse hook cannot afford to discover that
// for itself inside 150 ms, so the prefetch discovers it on the 5 s budget and
// hands over the answer along with which question it turned out to be.
func runSignpostFetch(cmd *cobra.Command, _ []string) error {
	if fetchID == "" || fetchQuery == "" {
		return nil
	}
	timeout := time.Duration(envInt("DP_SIGNPOST_PREFETCH_TIMEOUT_MS", 5000)) * time.Millisecond
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()
	intent := signpost.Intent{Family: fetchFamily, Query: fetchQuery, SymbolCandidate: fetchSymbol}
	if intent.Family == "" {
		intent.Family = signpost.FamilyLiteral
	}
	search := signpost.HTTPSearcher(env("DP_SIGNPOST_BOBBIN_URL", "http://localhost:3000/search"),
		os.Getenv("DP_SIGNPOST_REPO"), searchMode(), timeout)
	resolved, result, err := signpost.Resolve(ctx, intent, search)
	if err == nil {
		_ = signpost.WriteWarmResult(signpostCacheDir(), fetchID, resolved.Family, result)
	}
	return nil
}

// deliverParkedCorrection emits a correction parked by a previous failure, once.
// It writes nothing when there is none, which is the overwhelmingly common case,
// and it never fails the hook.
func deliverParkedCorrection(raw []byte) {
	var p struct {
		SessionID string `json:"session_id"`
	}
	if json.Unmarshal(raw, &p) != nil || p.SessionID == "" {
		return
	}
	ttl := time.Duration(envInt("DP_PAVE_PENDING_TTL_MS", int(pave.DefaultTTL/time.Millisecond))) * time.Millisecond
	pending, ok := pave.Take(pave.Dir(), p.SessionID, ttl)
	if !ok {
		return
	}
	var out struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	out.HookSpecificOutput.HookEventName = "PreToolUse"
	out.HookSpecificOutput.AdditionalContext = pending.Render()
	b, err := json.Marshal(out)
	if err != nil {
		return
	}
	_, _ = os.Stdout.Write(append(b, '\n'))
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
		TaskID: os.Getenv("DP_SIGNPOST_TASK_ID"), Model: os.Getenv("DP_SIGNPOST_MODEL_FAMILY"),
		Repo: os.Getenv("DP_SIGNPOST_REPO"), CacheDir: signpostCacheDir(),
		Payload:         os.Getenv("DP_SIGNPOST_PAYLOAD") == "1",
		PayloadHits:     envInt("DP_SIGNPOST_PAYLOAD_HITS", signpost.DefaultPayloadHits),
		PayloadMaxBytes: envInt("DP_SIGNPOST_PAYLOAD_MAX_BYTES", signpost.DefaultPayloadMaxBytes)}
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()
	out, event, err := signpost.Process(ctx, raw, cfg, signpost.HTTPSearcher(cfg.BobbinURL, cfg.Repo, searchMode(), timeout))
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

// searchMode selects the Bobbin search mode the hook asks for. Unset leaves the
// backend default in place.
//
// WHICH MODE IS RIGHT DEPENDS ON WHAT THE INDEX HOLDS, and the earlier
// measurement here was read as a general result when it was a corpus-specific
// one. On the Go and Rust evaluation corpora the modes are within ~5 ms of each
// other with identical candidate counts, so there was no case for overriding
// the default. On the fleet's resident index — docs, YAML and Jinja rather than
// source — they are not close: measured paired and interleaved, 16 pairs over
// two repositories and four queries, hybrid ran 164-197 ms on the heaviest
// repository against semantic's 51-70 ms, with identical candidate counts and
// the same result set in a different order. The keyword arm of hybrid is the
// entire difference. Against a 150 ms contract that is the difference between
// delivering and not, so the resident deployment pins semantic and says why.
func searchMode() string { return os.Getenv("DP_SIGNPOST_SEARCH_MODE") }

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
