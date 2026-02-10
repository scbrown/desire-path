package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// paveCheckCmd is the fast PreToolUse hook handler.
// It reads the hook payload from stdin, looks up the tool_name in aliases,
// and exits with code 2 (block) if an alias exists, or 0 (allow) otherwise.
var paveCheckCmd = &cobra.Command{
	Use:    "pave-check",
	Short:  "PreToolUse hook: check tool name against aliases (internal)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runPaveCheck(os.Stdin)
	},
}

func init() {
	rootCmd.AddCommand(paveCheckCmd)
}

// hookPayload is the minimal subset of the PreToolUse hook JSON we need.
type hookPayload struct {
	ToolName string `json:"tool_name"`
}

// runPaveCheck reads a hook payload from r, looks up the tool name,
// and exits 2 with a stderr message if an alias match is found.
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

	alias, err := s.GetAlias(context.Background(), payload.ToolName, "", "", "", "")
	if err != nil {
		// Lookup error → allow the call.
		return nil
	}
	if alias == nil {
		// No alias → allow.
		return nil
	}

	// Alias found → block the call. Exit code 2 tells Claude Code to
	// block the tool call and show our stderr message to the model.
	fmt.Fprintf(os.Stderr, "%s is not a valid tool. Use %s instead.", payload.ToolName, alias.To)
	os.Exit(2)
	return nil // unreachable
}
