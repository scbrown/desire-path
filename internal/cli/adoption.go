package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/scbrown/desire-path/internal/adoption"
	"github.com/scbrown/desire-path/internal/signpost"
	"github.com/spf13/cobra"
)

var adoptionPhase, adoptionStart, adoptionEnd, adoptionLog string

var adoptionHookCmd = &cobra.Command{
	Use: "adoption-hook", Short: "Observe prospective payload path use (hook internal)",
	Long: `Observe exact PreToolUse/PostToolUse invocation IDs and file operations.
Enabled only by DP_SIGNPOST_ADOPTION_DIR, which must be a private absolute directory.
DP_SIGNPOST_ADOPTION_ROOTS names a JSON file mapping index repository names to local
checkouts. Missing repository identity is unknown, never zero adoption. Responses,
commands, transcripts, CWDs and raw paths are not stored in this collector.
This command emits no hook context and never changes tool results.`,
	Example: `  DP_SIGNPOST_ADOPTION_DIR=/private/experiment dp adoption-hook --phase pre < hook.json`,
	RunE:    runAdoptionHook,
}

var adoptionReportCmd = &cobra.Command{
	Use: "adoption-report", Short: "Report a fixed prospective adoption window",
	Long: `Report completed paired outcomes and unknown, pending and unpaired observations.
--start and --end must name the fixed observed-install evaluation window as RFC3339
timestamps. --payload-log checks coverage against the original signpost JSONL with
a tolerant parser and reports malformed lines. Rates cover completed paired rows
only; missing/censored rows and an undefined zero-base ratio are not a verdict.
Nothing automatically disables payload injection.`,
	Example: `  DP_SIGNPOST_ADOPTION_DIR=/private/experiment dp adoption-report --start 2026-01-01T00:00:00Z --end 2026-01-08T00:00:00Z --payload-log offers.jsonl --json`,
	RunE:    runAdoptionReport,
}

func init() {
	adoptionHookCmd.Flags().StringVar(&adoptionPhase, "phase", "", "pre or post")
	adoptionReportCmd.Flags().StringVar(&adoptionStart, "start", "", "observed installation time (RFC3339)")
	adoptionReportCmd.Flags().StringVar(&adoptionEnd, "end", "", "fixed window end (RFC3339)")
	adoptionReportCmd.Flags().StringVar(&adoptionLog, "payload-log", "", "original offer JSONL for coverage/malformed counts")
	rootCmd.AddCommand(adoptionHookCmd, adoptionReportCmd)
}

func openAdoption(ctx context.Context) (*adoption.Store, error) {
	s, err := adoption.Open(ctx, os.Getenv("DP_SIGNPOST_ADOPTION_DIR"), adoption.Resolver{Scope: os.Getenv("DP_SIGNPOST_REPO")})
	if err != nil {
		return nil, err
	}
	roots, err := adoption.LoadRoots(os.Getenv("DP_SIGNPOST_ADOPTION_ROOTS"))
	if err != nil {
		_ = s.Invalid(ctx, "")
		s.Close()
		return nil, err
	}
	s.Resolver.Roots = roots
	return s, nil
}

func runAdoptionHook(cmd *cobra.Command, _ []string) (result error) {
	defer func() {
		if result != nil && os.Getenv("DP_SIGNPOST_ADOPTION_DIR") != "" {
			if adoption.MarkGap(os.Getenv("DP_SIGNPOST_ADOPTION_DIR")) != nil {
				fmt.Fprintln(os.Stderr, "adoption gap marker unavailable; collection cannot support a verdict")
			}
		}
	}()
	if os.Getenv("DP_SIGNPOST_ADOPTION_DIR") == "" {
		return nil
	}
	if adoptionPhase != "pre" && adoptionPhase != "post" {
		return errors.New("phase must be pre or post")
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 100*time.Millisecond)
	defer cancel()
	s, err := openAdoption(ctx)
	if err != nil {
		return errors.New("adoption observer unavailable; coverage is unknown")
	}
	defer s.Close()
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, (1<<20)+1))
	if err != nil {
		_ = s.Invalid(ctx, "")
		return errors.New("adoption input unavailable; coverage is unknown")
	}
	h, err := adoption.Parse(raw)
	if err != nil {
		_ = s.Invalid(ctx, h.Session)
		return errors.New("adoption input invalid; coverage is unknown")
	}
	if h.Event != "" && ((adoptionPhase == "pre" && h.Event != "PreToolUse") || (adoptionPhase == "post" && h.Event != "PostToolUse" && h.Event != "PostToolUseFailure")) {
		_ = s.Invalid(ctx, h.Session)
		return errors.New("adoption hook phase mismatch")
	}
	if adoptionPhase == "pre" {
		err = s.Start(ctx, h)
	} else {
		err = s.Finish(ctx, h)
	}
	if err != nil {
		return errors.New("adoption observation dropped; coverage is unknown")
	}
	return nil
}

func publishAdoption(ctx context.Context, raw []byte, event signpost.Event) {
	dropped := true
	defer func() {
		if dropped && os.Getenv("DP_SIGNPOST_ADOPTION_DIR") != "" {
			if adoption.MarkGap(os.Getenv("DP_SIGNPOST_ADOPTION_DIR")) != nil {
				fmt.Fprintln(os.Stderr, "adoption gap marker unavailable; collection cannot support a verdict")
			}
		}
	}()
	if os.Getenv("DP_SIGNPOST_ADOPTION_DIR") == "" {
		return
	}
	s, err := openAdoption(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "adoption offer observation dropped; coverage is unknown")
		return
	}
	defer s.Close()
	h, err := adoption.Parse(raw)
	if err != nil {
		_ = s.Invalid(ctx, h.Session)
		fmt.Fprintln(os.Stderr, "adoption offer context invalid; coverage is unknown")
		return
	}
	refs := make([]adoption.Reference, 0, len(event.PayloadRefs))
	for _, hit := range event.PayloadRefs {
		refs = append(refs, adoption.Reference{Repo: hit.Repo, Path: hit.Path})
	}
	if s.Publish(ctx, h, event.EventID, event.Timestamp, refs) != nil {
		fmt.Fprintln(os.Stderr, "adoption offer observation dropped; coverage is unknown")
		return
	}
	dropped = false
}

func runAdoptionReport(cmd *cobra.Command, _ []string) error {
	if os.Getenv("DP_SIGNPOST_ADOPTION_DIR") == "" {
		return errors.New("collector is not enabled")
	}
	start, err := time.Parse(time.RFC3339Nano, adoptionStart)
	if err != nil {
		return errors.New("start must be an observed-install RFC3339 timestamp")
	}
	end, err := time.Parse(time.RFC3339Nano, adoptionEnd)
	if err != nil || !end.After(start) {
		return errors.New("end must be a later RFC3339 timestamp")
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
	defer cancel()
	s, err := openAdoption(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	summary, err := s.Summary(ctx, start, end)
	if err != nil {
		return errors.New("collector report unavailable")
	}
	result := map[string]any{"start": start, "end": end, "window_elapsed": !time.Now().Before(end), "summary": summary,
		"measure":     "same-session explicit path operation in next ten started tool calls",
		"rates_scope": "completed paired observations only; not a verdict on unknown/censored rows"}
	if adoptionLog != "" {
		coverage, err := s.Coverage(ctx, adoptionLog, start, end)
		if err != nil {
			return err
		}
		result["coverage"] = coverage
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
