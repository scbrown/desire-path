package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/scbrown/desire-path/internal/cmdparse"
	"github.com/scbrown/desire-path/internal/model"
	"github.com/spf13/cobra"
)

// paveCheckCmd is the fast PreToolUse hook handler.
// Phase 1: blocks hallucinated tool names (exit 2 + stderr).
// Phase 2: rewrites tool parameters via updatedInput (exit 0 + JSON stdout).
var paveCheckCmd = &cobra.Command{
	Use:    "pave-check",
	Short:  "PreToolUse hook: check tool name and correct parameters (internal)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPaveCheck(os.Stdin)
	},
}

func init() {
	rootCmd.AddCommand(paveCheckCmd)
}

// hookPayload is the PreToolUse hook JSON from Claude Code.
type hookPayload struct {
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

// hookOutput is the JSON response for parameter corrections.
type hookOutput struct {
	HookSpecificOutput hookSpecific `json:"hookSpecificOutput"`
}

type hookSpecific struct {
	PermissionDecision string                 `json:"permissionDecision"`
	UpdatedInput       map[string]interface{} `json:"updatedInput,omitempty"`
	AdditionalContext  string                 `json:"additionalContext,omitempty"`
}

// runPaveCheck reads a hook payload from r, performs two phases of checking:
// 1. Tool-name alias → exit 2 (block)
// 2. Parameter correction rules → exit 0 with updatedInput JSON on stdout
func runPaveCheck(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	var payload hookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		// Can't parse → allow the call (don't block on hook errors).
		return nil
	}
	if payload.ToolName == "" {
		return nil
	}

	s, err := openStore()
	if err != nil {
		// Store unavailable → allow the call.
		return nil
	}
	defer s.Close()

	ctx := context.Background()

	// Phase 1: Tool-name alias check (block).
	alias, err := s.GetAlias(ctx, payload.ToolName, "", "", "", "")
	if err != nil {
		return nil // lookup error → allow
	}
	if alias != nil {
		msg := fmt.Sprintf("%s is not a valid tool. Use %s instead.", payload.ToolName, alias.To)
		if alias.Message != "" {
			msg = alias.Message
		}
		fmt.Fprint(os.Stderr, msg)
		os.Exit(2)
		return nil // unreachable
	}

	// Phase 2: Parameter correction rules.
	rules, err := s.GetRulesForTool(ctx, payload.ToolName)
	if err != nil || len(rules) == 0 {
		return nil // no rules or error → allow
	}

	corrections := applyRules(payload.ToolInput, rules)
	if len(corrections) == 0 {
		return nil // no corrections needed → allow
	}

	// Build updatedInput with all corrections applied.
	updatedInput := make(map[string]interface{})
	var contextParts []string
	for _, c := range corrections {
		updatedInput[c.param] = c.newValue
		contextParts = append(contextParts, c.description)
	}

	out := hookOutput{
		HookSpecificOutput: hookSpecific{
			PermissionDecision: "allow",
			UpdatedInput:       updatedInput,
			AdditionalContext:  "Corrected: " + strings.Join(contextParts, "; "),
		},
	}

	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(out)
}

// correction represents a single parameter correction.
type correction struct {
	param       string
	newValue    string
	description string
}

// applyRules applies all matching rules to the tool input and returns corrections.
func applyRules(toolInput map[string]interface{}, rules []model.Alias) []correction {
	var corrections []correction

	// Group rules by param so we can compose multiple corrections on the same parameter.
	paramValues := make(map[string]string)

	for _, rule := range rules {
		paramName := rule.Param
		// Get current value (possibly already corrected by a previous rule).
		val, ok := paramValues[paramName]
		if !ok {
			raw, exists := toolInput[paramName]
			if !exists {
				continue
			}
			str, isStr := raw.(string)
			if !isStr {
				continue
			}
			val = str
		}

		corrected, desc, applied := applyRule(val, rule)
		if applied {
			paramValues[paramName] = corrected
			corrections = append(corrections, correction{
				param:       paramName,
				newValue:    corrected,
				description: desc,
			})
		}
	}

	// Deduplicate: only keep the last correction per param (it has all composed changes).
	final := make(map[string]correction)
	for _, c := range corrections {
		final[c.param] = c
	}

	// Collect descriptions.
	result := make([]correction, 0, len(final))
	for _, c := range final {
		// Merge descriptions from all corrections for this param.
		var descs []string
		for _, cc := range corrections {
			if cc.param == c.param {
				descs = append(descs, cc.description)
			}
		}
		c.description = strings.Join(descs, "; ")
		result = append(result, c)
	}

	return result
}

// applyRule applies a single rule to a parameter value.
// Returns (corrected, description, applied).
func applyRule(value string, rule model.Alias) (string, string, bool) {
	switch rule.MatchKind {
	case "flag":
		return applyFlagRule(value, rule)
	case "command":
		return applyCommandRule(value, rule)
	case "literal":
		return applyLiteralRule(value, rule)
	case "regex":
		return applyRegexRule(value, rule)
	default:
		return "", "", false
	}
}

func applyFlagRule(value string, rule model.Alias) (string, string, bool) {
	segs := cmdparse.Parse(value)
	for _, seg := range segs {
		if seg.Command != rule.Command {
			continue
		}
		corrected, ok := cmdparse.CorrectFlag(seg, rule.From, rule.To)
		if ok {
			full := cmdparse.ApplyToFull(value, seg, corrected)
			desc := fmt.Sprintf("-%s → -%s", rule.From, rule.To)
			if rule.Message != "" {
				desc = rule.Message
			}
			return full, desc, true
		}
	}
	return "", "", false
}

func applyCommandRule(value string, rule model.Alias) (string, string, bool) {
	segs := cmdparse.Parse(value)
	for _, seg := range segs {
		if seg.Command != rule.Command {
			continue
		}
		corrected := cmdparse.SubstituteCommand(seg, rule.To)
		full := cmdparse.ApplyToFull(value, seg, corrected)
		desc := fmt.Sprintf("%s → %s", rule.From, rule.To)
		if rule.Message != "" {
			desc = rule.Message
		}
		return full, desc, true
	}
	return "", "", false
}

func applyLiteralRule(value string, rule model.Alias) (string, string, bool) {
	segs := cmdparse.Parse(value)
	for _, seg := range segs {
		if seg.Command != rule.Command {
			continue
		}
		if !strings.Contains(seg.Raw, rule.From) {
			continue
		}
		corrected := cmdparse.ReplaceLiteral(seg, rule.From, rule.To)
		full := cmdparse.ApplyToFull(value, seg, corrected)
		desc := fmt.Sprintf("%s → %s", rule.From, rule.To)
		if rule.Message != "" {
			desc = rule.Message
		}
		return full, desc, true
	}
	return "", "", false
}

func applyRegexRule(value string, rule model.Alias) (string, string, bool) {
	re, err := regexp.Compile(rule.From)
	if err != nil {
		return "", "", false // bad regex → skip
	}
	if !re.MatchString(value) {
		return "", "", false
	}
	corrected := re.ReplaceAllString(value, rule.To)
	desc := fmt.Sprintf("regex: %s → %s", rule.From, rule.To)
	if rule.Message != "" {
		desc = rule.Message
	}
	return corrected, desc, true
}
