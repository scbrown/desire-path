package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
	"github.com/spf13/cobra"
)

var (
	mapTool    string
	mapExcerpt string
	mapDelete  string
)

var mapCmd = &cobra.Command{
	Use:   "map <pattern> --doc <path>",
	Short: "Create a doc mapping linking failure patterns to documentation",
	Long: `Map documentation to a tool failure pattern. When agents struggle
with a tool, matching doc mappings surface relevant documentation.

The pattern can be a tool name, error substring, or glob pattern (using *).`,
	Example: `  dp map "gt mail send" --doc ~/gt/gastown/docs/mail.md
  dp map "unknown flag.*--assign" --tool Bash --doc "Use --assignee (-a) not --assign" --excerpt
  dp map --delete <id>`,
	RunE: runMap,
}

var mappingsCmd = &cobra.Command{
	Use:     "mappings",
	Short:   "List all doc mappings",
	Example: `  dp mappings
  dp mappings --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listMappings()
	},
}

var suggestCmd = &cobra.Command{
	Use:   "suggest [--tool <name>] [--error <text>]",
	Short: "Find documentation for a failing tool or error",
	Long:  `Query doc mappings to find relevant documentation for a tool failure.`,
	Example: `  dp suggest --tool Bash --error "unknown flag: --assign"
  dp suggest --tool "gt mail send"`,
	RunE: runSuggest,
}

var (
	suggestTool  string
	suggestError string
)

var (
	strugglingSession  string
	strugglingMinFails int
	strugglingSince    string
)

var strugglingCmd = &cobra.Command{
	Use:   "struggling",
	Short: "Identify tools with high failure rates that need documentation",
	Long: `Analyze invocation data to find tools that agents struggle with.
A tool is "struggling" when it has 3+ failures. Results are ranked by failure rate.`,
	Example: `  dp struggling
  dp struggling --session <session-id>
  dp struggling --since 7d --json`,
	RunE: runStruggling,
}

var docPath string

func init() {
	mapCmd.Flags().StringVar(&docPath, "doc", "", "path to doc file or URL")
	mapCmd.Flags().StringVar(&mapTool, "tool", "", "filter to specific tool name")
	mapCmd.Flags().BoolVar(new(bool), "excerpt", false, "treat --doc value as inline excerpt instead of file path")
	mapCmd.Flags().StringVar(&mapDelete, "delete", "", "delete mapping by ID")

	suggestCmd.Flags().StringVar(&suggestTool, "tool", "", "tool name to find docs for")
	suggestCmd.Flags().StringVar(&suggestError, "error", "", "error text to match against")

	strugglingCmd.Flags().StringVar(&strugglingSession, "session", "", "filter to specific session")
	strugglingCmd.Flags().IntVar(&strugglingMinFails, "min-fails", 3, "minimum failures to qualify")
	strugglingCmd.Flags().StringVar(&strugglingSince, "since", "", "time filter (e.g., 7d, 30d)")

	rootCmd.AddCommand(mapCmd)
	rootCmd.AddCommand(mappingsCmd)
	rootCmd.AddCommand(suggestCmd)
	rootCmd.AddCommand(strugglingCmd)
}

func runMap(cmd *cobra.Command, args []string) error {
	if mapDelete != "" {
		return deleteMapping(mapDelete)
	}

	if len(args) < 1 {
		return fmt.Errorf("requires a pattern argument")
	}
	if docPath == "" {
		return fmt.Errorf("--doc is required")
	}

	pattern := args[0]

	// Generate deterministic ID from pattern + tool
	h := sha256.Sum256([]byte(pattern + "|" + mapTool))
	id := fmt.Sprintf("dm-%x", h[:4])

	excerpt := ""
	isExcerpt, _ := cmd.Flags().GetBool("excerpt")
	if isExcerpt {
		excerpt = docPath
		if len(excerpt) > 500 {
			excerpt = excerpt[:500]
		}
	}

	dm := model.DocMapping{
		ID:         id,
		Pattern:    pattern,
		Tool:       mapTool,
		DocPath:    docPath,
		DocExcerpt: excerpt,
	}

	s, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	if err := s.SetDocMapping(context.Background(), dm); err != nil {
		return fmt.Errorf("set doc mapping: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]string{
			"action":  "set",
			"id":      id,
			"pattern": pattern,
		})
	}
	fmt.Printf("Doc mapping set: %s -> %s (id: %s)\n", pattern, docPath, id)
	return nil
}

func deleteMapping(id string) error {
	s, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	deleted, err := s.DeleteDocMapping(context.Background(), id)
	if err != nil {
		return fmt.Errorf("delete doc mapping: %w", err)
	}
	if !deleted {
		return fmt.Errorf("mapping %q not found", id)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]string{"action": "deleted", "id": id})
	}
	fmt.Printf("Doc mapping deleted: %s\n", id)
	return nil
}

func listMappings() error {
	s, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	mappings, err := s.GetDocMappings(context.Background())
	if err != nil {
		return fmt.Errorf("get doc mappings: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if mappings == nil {
			mappings = []model.DocMapping{}
		}
		return enc.Encode(mappings)
	}

	if len(mappings) == 0 {
		fmt.Fprintln(os.Stderr, "No doc mappings configured.")
		return nil
	}

	tbl := NewTable(os.Stdout, "ID", "PATTERN", "TOOL", "DOC", "HITS", "UPDATED")
	for _, dm := range mappings {
		doc := dm.DocPath
		if len(doc) > 40 {
			doc = "..." + doc[len(doc)-37:]
		}
		tbl.Row(dm.ID, dm.Pattern, dm.Tool, doc, fmt.Sprintf("%d", dm.MatchCount), dm.UpdatedAt.Format("2006-01-02"))
	}
	return tbl.Flush()
}

func runSuggest(cmd *cobra.Command, args []string) error {
	if suggestTool == "" && suggestError == "" {
		return fmt.Errorf("at least one of --tool or --error is required")
	}

	s, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	mappings, err := s.SuggestDocs(context.Background(), suggestTool, suggestError)
	if err != nil {
		return fmt.Errorf("suggest docs: %w", err)
	}

	// Increment match counts for returned mappings
	for _, dm := range mappings {
		_ = s.IncrementDocMatchCount(context.Background(), dm.ID)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if mappings == nil {
			mappings = []model.DocMapping{}
		}
		return enc.Encode(mappings)
	}

	if len(mappings) == 0 {
		fmt.Fprintln(os.Stderr, "No matching documentation found.")
		return nil
	}

	for _, dm := range mappings {
		fmt.Printf("--- %s [%s] ---\n", dm.Pattern, dm.ID)
		if dm.Tool != "" {
			fmt.Printf("  Tool: %s\n", dm.Tool)
		}
		fmt.Printf("  Doc:  %s\n", dm.DocPath)
		if dm.DocExcerpt != "" {
			fmt.Printf("  %s\n", dm.DocExcerpt)
		}
		fmt.Println()
	}
	return nil
}

func runStruggling(cmd *cobra.Command, args []string) error {
	s, err := openStore()
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer s.Close()

	opts := store.StrugglingOpts{
		MinFails:  strugglingMinFails,
		SessionID: strugglingSession,
	}
	if strugglingSince != "" {
		opts.Since = parseSinceDuration(strugglingSince)
	}

	tools, err := s.StrugglingTools(context.Background(), opts)
	if err != nil {
		return fmt.Errorf("struggling tools: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if tools == nil {
			tools = []model.StrugglingTool{}
		}
		return enc.Encode(tools)
	}

	if len(tools) == 0 {
		fmt.Fprintln(os.Stderr, "No struggling tools found.")
		return nil
	}

	tbl := NewTable(os.Stdout, "TOOL", "FAILS", "TOTAL", "RATE", "SESSIONS", "HAS DOC")
	for _, t := range tools {
		hasDoc := ""
		if t.HasDoc {
			hasDoc = "yes"
		}
		tbl.Row(t.ToolName, fmt.Sprintf("%d", t.Failures), fmt.Sprintf("%d", t.Total),
			fmt.Sprintf("%.0f%%", t.FailureRate*100), fmt.Sprintf("%d", t.Sessions), hasDoc)
	}
	return tbl.Flush()
}

// parseSinceDuration parses a duration string like "7d", "30d", "24h" into
// a time.Time representing that duration ago from now.
func parseSinceDuration(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	// Try standard duration first
	if d, err := time.ParseDuration(s); err == nil {
		return time.Now().Add(-d)
	}
	// Try Nd format (days)
	if strings.HasSuffix(s, "d") {
		s = strings.TrimSuffix(s, "d")
		var days int
		if _, err := fmt.Sscanf(s, "%d", &days); err == nil {
			return time.Now().AddDate(0, 0, -days)
		}
	}
	return time.Time{}
}
