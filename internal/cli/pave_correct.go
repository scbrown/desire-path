package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/scbrown/desire-path/internal/config"
	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/pave"
	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

// paveCorrectCmd is the PostToolUseFailure hook handler.
// When auto_correct is enabled and a failed command matches an alias rule,
// it re-executes the corrected command and returns the result.
var paveCorrectCmd = &cobra.Command{
	Use:    "pave-correct",
	Short:  "PostToolUseFailure hook: auto-correct and re-execute failed commands (internal)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPaveCorrect(os.Stdin)
	},
}

func init() {
	rootCmd.AddCommand(paveCorrectCmd)
}

// postFailurePayload is the PostToolUseFailure hook JSON from Claude Code.
type postFailurePayload struct {
	SessionID string                 `json:"session_id"`
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
	Error     string                 `json:"error"`
}

// postFailureOutput is the JSON response for PostToolUseFailure hooks.
type postFailureOutput struct {
	HookSpecificOutput postFailureSpecific `json:"hookSpecificOutput"`
}

type postFailureSpecific struct {
	AdditionalContext string `json:"additionalContext,omitempty"`
}

// autoCorrectTimeout is the maximum time to wait for a corrected command.
const autoCorrectTimeout = 30 * time.Second

// runPaveCorrect reads a PostToolUseFailure payload, checks alias rules for
// corrections, and optionally re-executes the corrected command.
// Also checks doc-mappings for relevant help documentation and records metrics.
func runPaveCorrect(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil // read error -> allow silently
	}

	var payload postFailurePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	if payload.ToolName == "" {
		return nil
	}

	s, err := openStore()
	if err != nil {
		return nil
	}
	defer s.Close()

	ctx := context.Background()

	var corrections []correction
	var correctedCmd string

	// Check tool-specific parameter rules (Bash commands, MCP tool params, etc.)
	rules, err := s.GetRulesForTool(ctx, payload.ToolName)
	if err == nil && len(rules) > 0 {
		corrections = applyRules(payload.ToolInput, rules)
	}

	// Also check tool-name aliases: if the tool itself is wrong, suggest the correct one
	if len(corrections) == 0 {
		alias, err := s.GetAlias(ctx, payload.ToolName, "", "", "", "")
		if err == nil && alias != nil {
			corrections = append(corrections, correction{
				param:       "_tool_name",
				newValue:    alias.To,
				description: fmt.Sprintf("Use %s instead of %s", alias.To, payload.ToolName),
			})
		}
	}

	// Build context parts
	var contextParts []string

	// Part 1: Alias-based corrections
	if len(corrections) > 0 {
		var correctionDescs []string
		for _, c := range corrections {
			correctionDescs = append(correctionDescs, c.description)
			if c.param == "command" {
				correctedCmd = c.newValue
			}
		}
		contextParts = append(contextParts,
			"**Auto-correction available:** "+strings.Join(correctionDescs, "; "))

		// Record correction metrics
		recordCorrections(s, ctx, payload, corrections)
	}

	// Part 2: Doc-mapping hints — surface relevant documentation for the failure
	docHints := findDocHints(s, ctx, payload)
	if docHints != "" {
		contextParts = append(contextParts, docHints)
	}

	// If no corrections and no doc hints, nothing to inject
	if len(contextParts) == 0 {
		return nil
	}

	correctionMsg := strings.Join(contextParts, "\n\n")

	// Check if auto_correct is enabled.
	// (deferred delivery is arranged after the message is final, below)
	cfg, cfgErr := config.LoadFrom(configPath)
	autoCorrect := cfgErr == nil && cfg.AutoCorrect

	if !autoCorrect || correctedCmd == "" {
		// Inject-only mode: tell the agent what the correct command is.
		if correctedCmd != "" {
			correctionMsg += fmt.Sprintf("\n\n**Corrected command:** `%s`", correctedCmd)
		}
		deferCorrection(payload, correctionMsg)
		out := postFailureOutput{
			HookSpecificOutput: postFailureSpecific{
				AdditionalContext: correctionMsg,
			},
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}

	// Auto-correct mode: execute the corrected command.
	result, execErr := executeCommand(correctedCmd)

	var sb strings.Builder
	sb.WriteString(correctionMsg)
	sb.WriteString("\n\n**Auto-executed corrected command.**")
	if execErr != nil {
		sb.WriteString(fmt.Sprintf("\nExecution error: %s", execErr))
	}
	if result != "" {
		// Cap output to avoid overwhelming the context.
		if len(result) > 4096 {
			result = result[:4096] + "\n... (output truncated)"
		}
		sb.WriteString(fmt.Sprintf("\nResult:\n```\n%s\n```", result))
	}

	deferCorrection(payload, sb.String())
	out := postFailureOutput{
		HookSpecificOutput: postFailureSpecific{
			AdditionalContext: sb.String(),
		},
	}
	return json.NewEncoder(os.Stdout).Encode(out)
}

// deferCorrection stores the correction for delivery on the agent's next
// PreToolUse call.
//
// The additionalContext returned alongside it is NOT redundant belt-and-braces:
// it is the correct output for this hook, and it is what a harness that
// surfaces PostToolUseFailure context would use. On Claude Code that text is
// discarded (aegis-c2g1s5), which is why the same message is also parked here
// and injected one turn later, where delivery is proven. Neither path is a
// fallback for the other; they serve different harnesses.
//
// Failures here are swallowed deliberately: a hook that cannot park a
// correction must still not disturb the tool call it observed.
func deferCorrection(payload postFailurePayload, message string) {
	command, _ := payload.ToolInput["command"].(string)
	_ = pave.Store(pave.Dir(), payload.SessionID, command, message)
}

// executeCommand runs a shell command and returns combined stdout+stderr output.
func executeCommand(command string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), autoCorrectTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// recordCorrections records correction events in the desires DB for metrics.
func recordCorrections(s store.Store, ctx context.Context, payload postFailurePayload, corrections []correction) {
	for _, c := range corrections {
		metadata, _ := json.Marshal(map[string]string{
			"from":  c.description,
			"to":    c.newValue,
			"param": c.param,
		})
		d := model.Desire{
			ID:        fmt.Sprintf("correct-%d", time.Now().UnixNano()),
			ToolName:  payload.ToolName,
			Error:     payload.Error,
			Category:  "correction",
			Source:    "pave-correct",
			Timestamp: time.Now(),
			Metadata:  metadata,
		}
		_ = s.RecordDesire(ctx, d)
	}
}

// findDocHints checks doc-mappings for relevant help documentation.
func findDocHints(s store.Store, ctx context.Context, payload postFailurePayload) string {
	mappings, err := s.GetDocMappings(ctx)
	if err != nil || len(mappings) == 0 {
		return ""
	}

	var hints []string
	for _, m := range mappings {
		// Match by tool name or error pattern
		if m.Tool != "" && m.Tool != payload.ToolName {
			continue
		}
		if m.Pattern != "" && !strings.Contains(payload.Error, m.Pattern) &&
			!strings.Contains(payload.ToolName, m.Pattern) {
			continue
		}

		hint := fmt.Sprintf("- **%s**", m.Pattern)
		if m.DocExcerpt != "" {
			hint += ": " + m.DocExcerpt
		}
		if m.DocPath != "" {
			hint += fmt.Sprintf(" (see `%s`)", m.DocPath)
		}
		hints = append(hints, hint)

		// Increment match count (best effort)
		m.MatchCount++
		_ = s.SetDocMapping(ctx, m)
	}

	if len(hints) == 0 {
		return ""
	}
	return "**Related documentation:**\n" + strings.Join(hints, "\n")
}
