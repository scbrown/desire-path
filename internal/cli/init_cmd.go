package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scbrown/desire-path/internal/source"
	"github.com/spf13/cobra"
)

var (
	initSource       string
	initClaudeCode   bool
	initTrackAll     bool
	initSettings     string
	initSignpostURL  string
	initSignpostLog  string
	initSearchMode   string
	initSkipURLCheck bool
)

// initCmd configures integration with AI coding tools.
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Set up integration with AI coding tools",
	Long: `Init configures dp to automatically record all tool calls from AI coding
assistants. Use --source to specify which tool to configure.

A single dp ingest hook is installed for both PostToolUse and
PostToolUseFailure events. The ingest pipeline dual-writes failures
to both the invocations and desires tables, so all data is captured
through a single entry point.

The command delegates to the source plugin's installer, which merges
configuration into the tool's settings file without overwriting existing
hooks or other configuration.

--signpost-url installs the environment the signpost hook needs, alongside the
hook itself. It is VERIFIED before it is written: an unreachable backend is
refused. That check is not a nicety — the signpost hook is silent when it cannot
reach a backend, because silence is its contract, so a hook pointed at nothing
looks exactly like a hook with nothing to say. Installed without this, it falls
back to a localhost default and can run on every pane of a fleet for weeks
delivering nothing. Pass --skip-signpost-url-check only when deliberately
installing ahead of a backend that is not up yet.

Environment is installed even when the hooks are already present, because a
correct hook set with no environment is precisely the broken state this
flag exists to repair.`,
	Example: `  dp init --source claude-code
  dp init --source claude-code --signpost-url http://127.0.0.1:3030`,
	// A RUNTIME failure here — a backend that does not answer — must not render
	// as a usage error. Cobra prints the usage blurb on any RunE error by
	// default, which reads as "you called me wrong" and sends the reader to fix
	// their flags instead of their backend.
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Handle deprecated --claude-code flag as alias.
		if initClaudeCode {
			if initSource != "" && initSource != "claude-code" {
				return fmt.Errorf("--claude-code conflicts with --source %s", initSource)
			}
			initSource = "claude-code"
		}

		if initSource == "" {
			names := source.Names()
			if len(names) == 0 {
				return fmt.Errorf("specify a source with --source NAME")
			}
			return fmt.Errorf("specify a source with --source NAME (available: %s)", strings.Join(names, ", "))
		}

		env, err := signpostEnv(initSignpostURL, initSignpostLog, initSearchMode, initSkipURLCheck)
		if err != nil {
			return err
		}
		return runInit(initSource, initTrackAll, initSettings, env)
	},
}

func init() {
	initCmd.Flags().StringVar(&initSource, "source", "", "source plugin to configure (e.g., claude-code)")
	initCmd.Flags().BoolVar(&initTrackAll, "track-all", false, "all invocations are now tracked by default (no-op)")
	initCmd.Flags().MarkDeprecated("track-all", "all invocations are now tracked by default")
	initCmd.Flags().BoolVar(&initClaudeCode, "claude-code", false, "configure Claude Code integration (deprecated: use --source claude-code)")
	initCmd.Flags().MarkDeprecated("claude-code", "use --source claude-code instead")
	initCmd.Flags().StringVar(&initSettings, "settings", "", "path to settings file (default: source-specific)")
	initCmd.Flags().StringVar(&initSignpostURL, "signpost-url", "", "base URL of the search backend the signpost hook should query (its /search route is derived); verified before it is written")
	initCmd.Flags().StringVar(&initSignpostLog, "signpost-log", "", "path for the signpost event log (default ~/.local/log/dp-signpost-events.jsonl; \"-\" disables it)")
	initCmd.Flags().StringVar(&initSearchMode, "signpost-search-mode", "", "search mode the signpost hook asks for: hybrid, semantic or keyword (default: the backend's own default)")
	initCmd.Flags().BoolVar(&initSkipURLCheck, "skip-signpost-url-check", false, "write --signpost-url without verifying it answers (for installing against a backend that is not up yet)")
	rootCmd.AddCommand(initCmd)
}

// signpostEnv turns --signpost-url into the environment the signpost hook needs,
// after CHECKING that the URL answers.
//
// The check is the point. `dp signpost` falls back to a localhost default and
// stays silent when it cannot reach a backend — silence is the contract, so a
// hook pointed at nothing is indistinguishable from a hook with nothing to say.
// That configuration ran on every pane of a fleet for weeks. An install is the
// one moment where the mistake is both detectable and cheap to fix, so it fails
// closed here rather than being discovered by measuring adoption later.
func signpostEnv(rawURL, logPath, searchMode string, skipCheck bool) (map[string]string, error) {
	if rawURL == "" {
		return nil, nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("--signpost-url %q is not an absolute URL", rawURL)
	}
	route := strings.TrimSuffix(u.String(), "/")
	if !strings.HasSuffix(route, "/search") {
		route += "/search"
	}
	if !skipCheck {
		if err := checkSignpostBackend(route); err != nil {
			return nil, fmt.Errorf("%w\n\nThe hook would be installed pointing at a backend that does not answer, and it "+
				"would then be SILENT rather than failing — which is how this went unnoticed on a whole fleet. "+
				"Bring the backend up, or pass --skip-signpost-url-check if you are installing ahead of it", err)
		}
	}
	env := map[string]string{"DP_SIGNPOST_BOBBIN_URL": route}

	// An event log in PRODUCTION, not only in the evaluation harness.
	//
	// Every claim about signposting so far comes from a harness, because the
	// deployed hook wrote no events at all: there was no way to ask what the
	// fleet actually saw, which is why "wired on every pane" survived for weeks
	// as a statement about delivery when it was only ever a statement about
	// installation. One line per eligible search is cheap; not being able to
	// answer the question is not.
	if logPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("determine home directory for the signpost event log: %w", err)
		}
		logPath = filepath.Join(home, ".local", "log", "dp-signpost-events.jsonl")
	}
	if logPath != "-" {
		env["DP_SIGNPOST_LOG"] = logPath
	}

	// The search mode is a property of the INDEX's content, not a global
	// preference, which is why it is a flag and not a default.
	//
	// On Go and Rust source the two modes are within ~5 ms of each other, and
	// the shipped hook states no mode at all. On a resident index holding
	// docs-and-IaC repositories they are not close: measured paired and
	// interleaved, 16 pairs over two repositories and four queries, hybrid ran
	// 164-197 ms against semantic's 51-70 ms on the heaviest repo, with
	// IDENTICAL candidate counts and the same result set in a different order.
	// The keyword arm of hybrid is the whole difference. At 150 ms that is not
	// a tuning preference, it is the difference between delivering and not.
	switch searchMode {
	case "":
	case "hybrid", "semantic", "keyword":
		env["DP_SIGNPOST_SEARCH_MODE"] = searchMode
	default:
		return nil, fmt.Errorf("--signpost-search-mode %q is not one of hybrid, semantic, keyword", searchMode)
	}
	return env, nil
}

// checkSignpostBackend asks the route a real question. The budget is generous
// on purpose: this checks REACHABILITY, and a backend that answers slowly is a
// different problem from one that is not there — one the eval harness measures
// and this command must not silently conflate with it.
func checkSignpostBackend(route string) error {
	u, err := url.Parse(route)
	if err != nil {
		return err
	}
	q := u.Query()
	q.Set("q", "signpost install probe")
	q.Set("limit", "1")
	u.RawQuery = q.Encode()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(u.String())
	if err != nil {
		return fmt.Errorf("signpost backend %s did not answer: %w", route, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("signpost backend %s answered HTTP %d", route, resp.StatusCode)
	}
	var probe struct {
		Count *int `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&probe); err != nil || probe.Count == nil {
		return fmt.Errorf("signpost backend %s answered, but not with a search result", route)
	}
	return nil
}

// runInit looks up the named source plugin, checks if it supports
// installation, and delegates to its Install method.
func runInit(name string, trackAll bool, settingsPath string, env map[string]string) error {
	src := source.Get(name)
	if src == nil {
		names := source.Names()
		if len(names) == 0 {
			return fmt.Errorf("unknown source %q", name)
		}
		return fmt.Errorf("unknown source %q (available: %s)", name, strings.Join(names, ", "))
	}

	installer, ok := src.(source.Installer)
	if !ok {
		return fmt.Errorf("source %q does not support auto-install", name)
	}

	// Check if hooks are already configured for idempotency. Environment is
	// installed even when the hooks are already there: the fleet case this
	// exists for is exactly a correct hook with no environment, and reporting
	// "already configured" would leave it that way.
	configDir := ""
	if settingsPath != "" {
		configDir = filepath.Dir(settingsPath)
	}
	installed, err := installer.IsInstalled(configDir)
	if err == nil && installed && len(env) == 0 {
		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(map[string]interface{}{
				"status": "already_configured",
				"source": name,
			})
		}
		fmt.Fprintf(os.Stdout, "hooks already configured for %s\n", name)
		return nil
	}

	opts := source.InstallOpts{SettingsPath: settingsPath, TrackAll: trackAll, Env: env}
	if err := installer.Install(opts); err != nil {
		return err
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]interface{}{
			"status":    "configured",
			"source":    name,
			"track_all": trackAll,
		})
	}
	fmt.Fprintf(os.Stdout, "Source %q integration configured!\n", name)
	fmt.Fprintf(os.Stdout, "All tool invocations will be recorded (PostToolUse + PostToolUseFailure).\n")
	fmt.Fprintf(os.Stdout, "View them with:   dp list\n")
	fmt.Fprintf(os.Stdout, "Analyze patterns: dp paths\n")
	return nil
}
