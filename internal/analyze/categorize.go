package analyze

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/scbrown/desire-path/internal/model"
)

// envNeedPatterns matches error messages indicating a missing command.
var envNeedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)command not found`),
	regexp.MustCompile(`(?:bash|sh|/bin/\w+):\s+\S+:\s+not found`),
	regexp.MustCompile(`(?i)no such file or directory`),
	regexp.MustCompile(`exit (?:code |status )?127\b`),
	regexp.MustCompile(`(?i)not found in PATH`),
	regexp.MustCompile(`(?i)not installed`),
}

// Command extraction patterns, precompiled for reuse.
var (
	reShellNotFound  = regexp.MustCompile(`(?:bash|sh|/bin/\w+):\s+(\S+):\s+(?:command )?not found`)
	reCmdNotFound    = regexp.MustCompile(`command not found:\s+(\S+)`)
	reNotFoundInPath = regexp.MustCompile(`(\S+):\s+not found in PATH`)
)

// CategorizeDesire returns the category for a desire based on its error
// message and tool context. Returns empty string if no category matches.
func CategorizeDesire(toolName, errorMsg string, toolInput json.RawMessage) string {
	if isEnvNeed(toolName, errorMsg) {
		return model.CategoryEnvNeed
	}
	return ""
}

// isEnvNeed detects "command not found" style errors from Bash tool calls.
func isEnvNeed(toolName, errorMsg string) bool {
	if toolName != "Bash" {
		return false
	}
	for _, pat := range envNeedPatterns {
		if pat.MatchString(errorMsg) {
			return true
		}
	}
	return false
}

// EnvNeedCommand extracts the missing command name from an error message
// or from the Bash tool_input. Returns empty string if not determinable.
func EnvNeedCommand(errorMsg string, toolInput json.RawMessage) string {
	// Try to extract from error message patterns like:
	// "bash: cargo-insta: command not found"
	// "/bin/sh: cargo-nextest: not found"
	if cmd := extractFromError(errorMsg); cmd != "" {
		return cmd
	}

	// Fall back to extracting from tool_input command field.
	if cmd := extractFromToolInput(toolInput); cmd != "" {
		return cmd
	}

	return ""
}

// extractFromError parses "command not found" style error messages.
func extractFromError(errorMsg string) string {
	// Pattern: "bash: <cmd>: command not found" or "<shell>: <cmd>: not found"
	if m := reShellNotFound.FindStringSubmatch(errorMsg); len(m) >= 2 {
		return m[1]
	}

	// Pattern: "command not found: <cmd>"
	if m := reCmdNotFound.FindStringSubmatch(errorMsg); len(m) >= 2 {
		return m[1]
	}

	// Pattern: "<cmd>: not found in PATH"
	if m := reNotFoundInPath.FindStringSubmatch(errorMsg); len(m) >= 2 {
		return m[1]
	}

	return ""
}

// extractFromToolInput gets the first command token from Bash tool_input.
func extractFromToolInput(toolInput json.RawMessage) string {
	if len(toolInput) == 0 {
		return ""
	}

	var input struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(toolInput, &input); err != nil || input.Command == "" {
		return ""
	}

	// Get the first token (the command name).
	cmd := strings.TrimSpace(input.Command)
	// Skip env var assignments like FOO=bar cmd
	for {
		parts := strings.SplitN(cmd, " ", 2)
		if len(parts) == 0 {
			return ""
		}
		token := parts[0]
		if strings.Contains(token, "=") && !strings.HasPrefix(token, "-") {
			// env var assignment, skip
			if len(parts) < 2 {
				return ""
			}
			cmd = strings.TrimSpace(parts[1])
			continue
		}
		return token
	}
}
