//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGoldenPath exercises the full happy-path workflow end to end:
// init → ingest → paths → similar → alias → pave --hook → pave-check.
// This is the Tier 3 P1 integration test.
func TestGoldenPath(t *testing.T) {
	e := newEnv(t)

	// Step 1: dp init --source claude-code
	stdout, _ := e.mustRun(nil, "init", "--source", "claude-code",
		"--settings", e.settingsPath())
	if !strings.Contains(stdout, "configured") {
		t.Fatalf("init: expected 'configured', got:\n%s", stdout)
	}

	// Step 2: ingest several failed tool calls.
	tools := []struct {
		name, session, errMsg string
	}{
		{"read_file", "s1", "unknown tool"},
		{"read_file", "s1", "unknown tool"},
		{"read_file", "s2", "unknown tool"},
		{"run_tests", "s2", "tool not available"},
		{"write_file", "s3", "not found"},
	}
	for _, tc := range tools {
		payload := e.fixture(tc.name, tc.session, tc.errMsg)
		e.mustRun(payload, "ingest", "--source", "claude-code")
	}

	// Step 3: dp paths — verify aggregation.
	stdout, _ = e.mustRun(nil, "paths", "--json")
	var paths []struct {
		Pattern string `json:"pattern"`
		Count   int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(stdout), &paths); err != nil {
		t.Fatalf("parse paths JSON: %v", err)
	}
	if len(paths) < 3 {
		t.Fatalf("expected ≥3 paths, got %d", len(paths))
	}
	// read_file should be first (most frequent).
	if paths[0].Pattern != "read_file" {
		t.Errorf("top path = %q, want read_file", paths[0].Pattern)
	}
	if paths[0].Count != 3 {
		t.Errorf("read_file count = %d, want 3", paths[0].Count)
	}

	// Step 4: dp similar — find matching tool (use low threshold to ensure results).
	stdout, _ = e.mustRun(nil, "similar", "read_file", "--json", "--threshold", "0.1")
	var suggestion struct {
		Query       string `json:"query"`
		Suggestions []struct {
			Name  string  `json:"name"`
			Score float64 `json:"score"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(stdout), &suggestion); err != nil {
		t.Fatalf("parse suggest JSON: %v", err)
	}
	if len(suggestion.Suggestions) == 0 {
		t.Fatal("suggest returned no suggestions")
	}
	// "Read" should appear in the suggestions for "read_file".
	foundRead := false
	for _, s := range suggestion.Suggestions {
		if s.Name == "Read" {
			foundRead = true
			break
		}
	}
	if !foundRead {
		t.Errorf("Read not found in suggestions: %v", suggestion.Suggestions)
	}

	// Step 5: dp alias — create the mapping.
	e.mustRun(nil, "alias", "read_file", "Read")
	e.mustRun(nil, "alias", "run_tests", "Bash")

	// Step 6: dp pave --hook — install the intercept.
	e.mustRun(nil, "pave", "--hook", "--settings", e.settingsPath())

	// Verify both init and pave hooks coexist in settings.
	settings := e.readSettingsJSON()
	hooks := settingsHooks(t, settings)
	assertHookCommand(t, hooks, "PostToolUse", "dp ingest --source claude-code")
	assertHookCommand(t, hooks, "PreToolUse", "dp pave-check")

	// Step 7: dp pave-check — aliased tool blocked, unaliased allowed.
	payload, _ := json.Marshal(map[string]string{"tool_name": "read_file"})
	_, stderr, err := e.run(payload, "pave-check")
	if exitCode(err) != 2 {
		t.Fatalf("pave-check read_file: expected exit 2, got %d (stderr: %q)", exitCode(err), stderr)
	}
	if !strings.Contains(stderr, "Read") {
		t.Errorf("pave-check stderr should mention 'Read', got: %q", stderr)
	}

	// Unaliased tool should pass.
	payload, _ = json.Marshal(map[string]string{"tool_name": "Write"})
	_, _, err = e.run(payload, "pave-check")
	if exitCode(err) != 0 {
		t.Fatalf("pave-check Write: expected exit 0, got %d", exitCode(err))
	}
}

// TestAgentsMDGeneration verifies dp pave --agents-md outputs correct rules
// from alias data.
func TestAgentsMDGeneration(t *testing.T) {
	e := newEnv(t)

	// Set up aliases.
	e.mustRun(nil, "alias", "read_file", "Read")
	e.mustRun(nil, "alias", "run_tests", "Bash")

	// Generate rules to stdout.
	stdout, _ := e.mustRun(nil, "pave", "--agents-md")
	if !strings.Contains(stdout, "Tool Name Corrections") {
		t.Errorf("expected header, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Do NOT call `read_file`. Use `Read` instead.") {
		t.Errorf("missing read_file rule in:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Do NOT call `run_tests`. Use `Bash` instead.") {
		t.Errorf("missing run_tests rule in:\n%s", stdout)
	}

	// JSON mode should return structured data.
	stdout, _ = e.mustRun(nil, "pave", "--agents-md", "--json")
	var result struct {
		Status string   `json:"status"`
		Rules  []string `json:"rules"`
		Count  int      `json:"count"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse agents-md JSON: %v", err)
	}
	if result.Status != "generated" {
		t.Errorf("status = %q, want generated", result.Status)
	}
	if result.Count != 2 {
		t.Errorf("count = %d, want 2", result.Count)
	}
}

// TestAgentsMDNoAliases verifies dp pave --agents-md with no aliases.
func TestAgentsMDNoAliases(t *testing.T) {
	e := newEnv(t)

	// JSON mode with no aliases.
	stdout, _ := e.mustRun(nil, "pave", "--agents-md", "--json")
	var result struct {
		Status string   `json:"status"`
		Rules  []string `json:"rules"`
	}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse JSON: %v", err)
	}
	if result.Status != "no_aliases" {
		t.Errorf("status = %q, want no_aliases", result.Status)
	}
	if len(result.Rules) != 0 {
		t.Errorf("expected empty rules, got %d", len(result.Rules))
	}
}

// TestAgentsMDAppend verifies dp pave --agents-md --append writes to a file.
func TestAgentsMDAppend(t *testing.T) {
	e := newEnv(t)

	// Create alias.
	e.mustRun(nil, "alias", "read_file", "Read")

	// Seed an existing file.
	agentsPath := filepath.Join(e.home, "AGENTS.md")
	original := "# My Project Rules\n\nSome existing content.\n"
	if err := os.WriteFile(agentsPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}

	// Append rules.
	e.mustRun(nil, "pave", "--agents-md", "--append", agentsPath)

	// Read back and verify.
	data := e.readFile(agentsPath)
	if !strings.HasPrefix(data, original) {
		t.Error("original content was clobbered")
	}
	if !strings.Contains(data, "Do NOT call `read_file`. Use `Read` instead.") {
		t.Errorf("appended rules missing in:\n%s", data)
	}
}

// TestAgentsMDAppendCreatesFile verifies --append creates the file if missing.
func TestAgentsMDAppendCreatesFile(t *testing.T) {
	e := newEnv(t)

	e.mustRun(nil, "alias", "run_tests", "Bash")

	agentsPath := filepath.Join(e.home, "AGENTS.md")

	// File does not exist yet.
	e.mustRun(nil, "pave", "--agents-md", "--append", agentsPath)

	data := e.readFile(agentsPath)
	if !strings.Contains(data, "run_tests") {
		t.Errorf("expected run_tests rule in new file, got:\n%s", data)
	}
}

// TestBothFlags verifies dp pave --hook --agents-md works in a single call.
func TestBothFlags(t *testing.T) {
	e := newEnv(t)

	e.mustRun(nil, "alias", "read_file", "Read")
	e.writeSettings(map[string]interface{}{})

	stdout, _ := e.mustRun(nil, "pave", "--hook", "--agents-md",
		"--settings", e.settingsPath())

	// Hook should be installed.
	settings := e.readSettingsJSON()
	hooks := settingsHooks(t, settings)
	assertHookCommand(t, hooks, "PreToolUse", "dp pave-check")

	// Rules should be printed to stdout.
	if !strings.Contains(stdout, "Do NOT call `read_file`. Use `Read` instead.") {
		t.Errorf("expected rules in stdout, got:\n%s", stdout)
	}
}

// TestMultiSourceIngest verifies ingesting from multiple sources and
// filtering by source in list output. It is table-driven across
// [claude-code, kiro, codex] per Plan 005 Tier 3.
func TestMultiSourceIngest(t *testing.T) {
	e := newEnv(t)

	type ingestCase struct {
		source  string
		tool    string // expected tool_name in list output
		payload []byte
	}
	cases := []ingestCase{
		// Claude Code: uses tool_name, session_id, error fields.
		{"claude-code", "read_file", e.fixture("read_file", "cc-s1", "unknown tool")},
		{"claude-code", "run_tests", e.fixture("run_tests", "cc-s1", "tool not available")},
		{"claude-code", "read_file", e.fixture("read_file", "cc-s2", "unknown tool")},
		// Kiro: uses tool_name, cwd, tool_response.success=false for errors.
		{"kiro", "write", e.kiroFixture("write", "command not found")},
		{"kiro", "shell", e.kiroFixture("shell", "permission denied")},
		// Codex: uses item.completed with item.status="failed" for errors.
		{"codex", "command_execution", e.codexFixture("command_execution", "bad command")},
		{"codex", "file_change", e.codexFixture("file_change", "write failed")},
	}

	for _, tc := range cases {
		e.mustRun(tc.payload, "ingest", "--source", tc.source)
	}

	// List all — should see all 7 desires (3 claude-code + 2 kiro + 2 codex).
	stdout, _ := e.mustRun(nil, "list", "--json")
	var allDesires []struct {
		ToolName string `json:"tool_name"`
		Source   string `json:"source"`
	}
	if err := json.Unmarshal([]byte(stdout), &allDesires); err != nil {
		t.Fatalf("parse list JSON: %v", err)
	}
	if len(allDesires) != 7 {
		t.Fatalf("expected 7 desires total, got %d", len(allDesires))
	}

	// Verify all three sources are represented.
	sourceSet := make(map[string]bool)
	for _, d := range allDesires {
		sourceSet[d.Source] = true
	}
	for _, s := range []string{"claude-code", "kiro", "codex"} {
		if !sourceSet[s] {
			t.Errorf("source %q not found in desires", s)
		}
	}

	// Paths should show 6 unique tools:
	// read_file(3), run_tests(1), write(1), shell(1), command_execution(1), file_change(1).
	stdout, _ = e.mustRun(nil, "paths", "--json")
	var paths []struct {
		Pattern string `json:"pattern"`
		Count   int    `json:"count"`
	}
	if err := json.Unmarshal([]byte(stdout), &paths); err != nil {
		t.Fatalf("parse paths JSON: %v", err)
	}
	if len(paths) != 6 {
		t.Fatalf("expected 6 paths, got %d", len(paths))
	}
	// read_file should be first (most frequent with count=2).
	if paths[0].Pattern != "read_file" {
		t.Errorf("top path = %q, want read_file", paths[0].Pattern)
	}
	if paths[0].Count != 2 {
		t.Errorf("read_file count = %d, want 2", paths[0].Count)
	}
}

// TestIngestThenAliasThenPaveCheck is a focused pipeline test verifying
// that data flows correctly from ingest → alias → pave-check.
func TestIngestThenAliasThenPaveCheck(t *testing.T) {
	type testCase struct {
		name      string
		aliasFrom string
		aliasTo   string
		checkTool string
		wantExit  int
	}
	cases := []testCase{
		{
			name:      "aliased tool is blocked",
			aliasFrom: "read_file",
			aliasTo:   "Read",
			checkTool: "read_file",
			wantExit:  2,
		},
		{
			name:      "unaliased tool is allowed",
			aliasFrom: "read_file",
			aliasTo:   "Read",
			checkTool: "Bash",
			wantExit:  0,
		},
		{
			name:      "different alias target",
			aliasFrom: "run_tests",
			aliasTo:   "Bash",
			checkTool: "run_tests",
			wantExit:  2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)

			// Ingest a desire.
			payload := e.fixture(tc.aliasFrom, "test-session", "error")
			e.mustRun(payload, "ingest", "--source", "claude-code")

			// Create alias.
			e.mustRun(nil, "alias", tc.aliasFrom, tc.aliasTo)

			// Check.
			hookPayload, _ := json.Marshal(map[string]string{"tool_name": tc.checkTool})
			_, stderr, err := e.run(hookPayload, "pave-check")
			code := exitCode(err)
			if code != tc.wantExit {
				t.Fatalf("exit code = %d, want %d (stderr: %q)", code, tc.wantExit, stderr)
			}
		})
	}
}

// TestSimilarAfterAlias verifies that dp similar returns the existing alias
// rather than similarity-based suggestions when an alias exists.
func TestSimilarAfterAlias(t *testing.T) {
	e := newEnv(t)

	// Without alias: similar should return similarity results (low threshold).
	stdout, _ := e.mustRun(nil, "similar", "read_file", "--json", "--threshold", "0.1")
	var before struct {
		Query       string `json:"query"`
		Suggestions []struct {
			Name string `json:"name"`
		} `json:"suggestions"`
	}
	if err := json.Unmarshal([]byte(stdout), &before); err != nil {
		t.Fatalf("parse pre-alias suggest: %v", err)
	}
	if len(before.Suggestions) == 0 {
		t.Fatal("expected suggestions before alias (threshold=0.1)")
	}

	// Create alias.
	e.mustRun(nil, "alias", "read_file", "Read")

	// With alias: similar should return the alias directly.
	stdout, _ = e.mustRun(nil, "similar", "read_file", "--json")
	var after struct {
		Query string `json:"query"`
		Alias string `json:"alias"`
	}
	if err := json.Unmarshal([]byte(stdout), &after); err != nil {
		t.Fatalf("parse post-alias suggest: %v", err)
	}
	if after.Alias != "Read" {
		t.Errorf("alias = %q, want Read", after.Alias)
	}
}

// TestStatsAfterIngest verifies that dp stats reflects ingested data.
func TestStatsAfterIngest(t *testing.T) {
	e := newEnv(t)

	// Ingest a few desires.
	for _, tool := range []string{"read_file", "read_file", "run_tests"} {
		payload := e.fixture(tool, "s1", "error")
		e.mustRun(payload, "ingest", "--source", "claude-code")
	}

	stdout, _ := e.mustRun(nil, "stats", "--json")
	var stats struct {
		TotalDesires int `json:"total_desires"`
		UniquePaths  int `json:"unique_paths"`
	}
	if err := json.Unmarshal([]byte(stdout), &stats); err != nil {
		t.Fatalf("parse stats JSON: %v", err)
	}
	if stats.TotalDesires != 3 {
		t.Errorf("total_desires = %d, want 3", stats.TotalDesires)
	}
	if stats.UniquePaths != 2 {
		t.Errorf("unique_paths = %d, want 2", stats.UniquePaths)
	}
}

// TestPaveRequiresFlag verifies dp pave with no flags returns an error.
func TestPaveRequiresFlag(t *testing.T) {
	e := newEnv(t)

	_, stderr, err := e.run(nil, "pave")
	if exitCode(err) == 0 {
		t.Fatal("expected non-zero exit for pave with no flags")
	}
	if !strings.Contains(stderr, "--hook") && !strings.Contains(stderr, "--agents-md") {
		t.Errorf("expected error to mention flags, got: %q", stderr)
	}
}

// TestPaveAgentsMDAppendIsAdditive verifies that running dp pave --agents-md
// --append twice duplicates the rule block. This documents current behavior:
// append is blind (no deduplication). If deduplication is later added, this
// test will break and force a deliberate decision.
func TestPaveAgentsMDAppendIsAdditive(t *testing.T) {
	e := newEnv(t)

	// Create alias.
	e.mustRun(nil, "alias", "foo", "Bar")

	agentsPath := filepath.Join(e.home, "AGENTS.md")

	// First append.
	e.mustRun(nil, "pave", "--agents-md", "--append", agentsPath)

	// Second append — should duplicate the rule block.
	e.mustRun(nil, "pave", "--agents-md", "--append", agentsPath)

	// Read back and verify the rule appears twice.
	data := e.readFile(agentsPath)
	rule := "Do NOT call `foo`. Use `Bar` instead."
	first := strings.Index(data, rule)
	if first == -1 {
		t.Fatalf("rule not found in:\n%s", data)
	}
	second := strings.Index(data[first+len(rule):], rule)
	if second == -1 {
		t.Errorf("expected rule block to appear twice (blind append), but found only once in:\n%s", data)
	}
}
