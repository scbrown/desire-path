package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/model"
	"github.com/scbrown/desire-path/internal/store"
)

func TestPaveCheckMatchingAlias(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	// Seed an alias.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{From: "read_file", To: "Read"}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	// Simulate hook stdin with a matching tool name.
	payload := `{"tool_name":"read_file","tool_input":{"file_path":"/tmp/test"}}`
	stdin := strings.NewReader(payload)

	// runPaveCheck calls os.Exit(2) on match, so we test the logic
	// up to the point before exit by checking the store lookup directly.
	s2, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	alias, err := s2.GetAlias(context.Background(), "read_file", "", "", "", "")
	if err != nil {
		t.Fatalf("get alias: %v", err)
	}
	if alias == nil {
		t.Fatal("expected alias, got nil")
	}
	if alias.To != "Read" {
		t.Errorf("expected alias target 'Read', got %q", alias.To)
	}

	// Also verify the payload parses correctly.
	var p hookPayload
	if err := json.NewDecoder(stdin).Decode(&p); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if p.ToolName != "read_file" {
		t.Errorf("expected tool_name 'read_file', got %q", p.ToolName)
	}
}

func TestPaveCheckNoAlias(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	// Create empty store.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	// Simulate hook stdin with a tool name that has no alias.
	payload := `{"tool_name":"Read"}`
	stdin := strings.NewReader(payload)

	// runPaveCheck should return nil (allow the call).
	err = runPaveCheck(stdin)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestPaveCheckInvalidJSON(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	// Invalid JSON should not block the call.
	stdin := strings.NewReader("not json at all")
	err = runPaveCheck(stdin)
	if err != nil {
		t.Fatalf("expected nil error for invalid JSON, got: %v", err)
	}
}

func TestPaveCheckEmptyToolName(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	stdin := strings.NewReader(`{"tool_name":""}`)
	err = runPaveCheck(stdin)
	if err != nil {
		t.Fatalf("expected nil error for empty tool_name, got: %v", err)
	}
}

func TestPaveAgentsMD(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	// Seed aliases.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{From: "read_file", To: "Read"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{From: "search_files", To: "Grep"}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	jsonOutput = false
	paveHook = false
	paveAgentsMD = true
	paveAppend = ""
	defer func() { paveAgentsMD = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"pave", "--db", db, "--agents-md"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "# Tool Name Corrections") {
		t.Errorf("expected header, got: %s", output)
	}
	if !strings.Contains(output, "Do NOT call `read_file`. Use `Read` instead.") {
		t.Errorf("expected read_file rule, got: %s", output)
	}
	if !strings.Contains(output, "Do NOT call `search_files`. Use `Grep` instead.") {
		t.Errorf("expected search_files rule, got: %s", output)
	}
}

func TestPaveAgentsMDAppend(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	// Seed an alias.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{From: "run_tests", To: "Bash"}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	outFile := filepath.Join(t.TempDir(), "AGENTS.md")
	if err := os.WriteFile(outFile, []byte("# Existing Content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	dbPath = db
	jsonOutput = false
	paveHook = false
	paveAgentsMD = true
	paveAppend = outFile
	defer func() { paveAgentsMD = false; paveAppend = "" }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"pave", "--db", db, "--agents-md", "--append", outFile})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	stdout := buf.String()

	if !strings.Contains(stdout, "Appended 1 rules") {
		t.Errorf("expected append confirmation, got: %s", stdout)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "# Existing Content") {
		t.Error("existing content was lost")
	}
	if !strings.Contains(content, "Do NOT call `run_tests`. Use `Bash` instead.") {
		t.Errorf("expected appended rule, got: %s", content)
	}
}

func TestPaveAgentsMDJSON(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{From: "read_file", To: "Read"}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	jsonOutput = true
	paveHook = false
	paveAgentsMD = true
	paveAppend = ""
	defer func() { jsonOutput = false; paveAgentsMD = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"pave", "--db", db, "--json", "--agents-md"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("json unmarshal: %v\nOutput: %s", err, buf.String())
	}
	if result["status"] != "generated" {
		t.Errorf("expected status 'generated', got %v", result["status"])
	}
	count, ok := result["count"].(float64)
	if !ok || count != 1 {
		t.Errorf("expected count 1, got %v", result["count"])
	}
}

func TestPaveAgentsMDNoAliases(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	jsonOutput = false
	paveHook = false
	paveAgentsMD = true
	paveAppend = ""
	defer func() { paveAgentsMD = false }()

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	rootCmd.SetArgs([]string{"pave", "--db", db, "--agents-md"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stderr = oldStderr
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if !strings.Contains(output, "No aliases configured.") {
		t.Errorf("expected 'No aliases configured.', got: %s", output)
	}
}

func TestPaveAgentsMDMixed(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	// Seed a tool-name alias and command correction rules.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{From: "read_file", To: "Read"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "r", To: "R", Tool: "Bash", Param: "command", Command: "scp", MatchKind: "flag",
		Message: "scp uses -R for recursive",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "grep", To: "rg", Tool: "Bash", Param: "command", Command: "grep", MatchKind: "command",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	jsonOutput = false
	paveHook = false
	paveAgentsMD = true
	paveAppend = ""
	defer func() { paveAgentsMD = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"pave", "--db", db, "--agents-md"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Tool name section.
	if !strings.Contains(output, "# Tool Name Corrections") {
		t.Errorf("expected tool name header, got: %s", output)
	}
	if !strings.Contains(output, "Do NOT call `read_file`. Use `Read` instead.") {
		t.Errorf("expected read_file rule, got: %s", output)
	}

	// Command corrections section.
	if !strings.Contains(output, "# Command Corrections") {
		t.Errorf("expected command corrections header, got: %s", output)
	}
	if !strings.Contains(output, "## scp") {
		t.Errorf("expected scp section header, got: %s", output)
	}
	if !strings.Contains(output, "Flag `-r` should be `-R`") {
		t.Errorf("expected flag correction, got: %s", output)
	}
	if !strings.Contains(output, "scp uses -R for recursive") {
		t.Errorf("expected custom message, got: %s", output)
	}
	if !strings.Contains(output, "## grep → rg") {
		t.Errorf("expected grep→rg header, got: %s", output)
	}
	if !strings.Contains(output, "Use `rg` instead of `grep`") {
		t.Errorf("expected command substitution rule, got: %s", output)
	}
}

func TestPaveAgentsMDRecipe(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	// Tool-name alias (should still appear in output).
	if err := s.SetAlias(context.Background(), model.Alias{From: "read_file", To: "Read"}); err != nil {
		t.Fatal(err)
	}
	// Recipe with custom message.
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "gt await-signal", To: "while true; do\n  status=$(gt mol status 2>&1)\n  echo \"$status\"\n  if echo \"$status\" | grep -q \"signaled\"; then break; fi\n  sleep 5\ndone",
		Tool: "Bash", Param: "command", Command: "gt", MatchKind: "recipe",
		Message: "Poll gt mol status in a loop",
	}); err != nil {
		t.Fatal(err)
	}
	// Recipe without message (should use default).
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "gt convoy wait", To: "while true; do\n  gt convoy status\n  sleep 10\ndone",
		Tool: "Bash", Param: "command", Command: "gt", MatchKind: "recipe",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	jsonOutput = false
	paveHook = false
	paveAgentsMD = true
	paveAppend = ""
	defer func() { paveAgentsMD = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"pave", "--db", db, "--agents-md"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Tool name section should still be present.
	if !strings.Contains(output, "# Tool Name Corrections") {
		t.Errorf("expected tool name header, got: %s", output)
	}
	if !strings.Contains(output, "Do NOT call `read_file`. Use `Read` instead.") {
		t.Errorf("expected read_file rule, got: %s", output)
	}

	// Command corrections section should contain recipe rules.
	if !strings.Contains(output, "# Command Corrections") {
		t.Errorf("expected command corrections header, got: %s", output)
	}

	// Recipe with message: should use the message, not a script preview.
	if !strings.Contains(output, "Do NOT use `gt await-signal`. Poll gt mol status in a loop") {
		t.Errorf("expected recipe rule with message, got: %s", output)
	}

	// Recipe without message: should use default text.
	if !strings.Contains(output, "Do NOT use `gt convoy wait` — it does not exist and will be rewritten automatically.") {
		t.Errorf("expected recipe rule with default text, got: %s", output)
	}

	// Script content should NOT appear in the output.
	if strings.Contains(output, "while true") {
		t.Errorf("script content should not appear in agents-md output, got: %s", output)
	}
	if strings.Contains(output, "sleep 5") {
		t.Errorf("script content should not appear in agents-md output, got: %s", output)
	}
}

func TestPaveNoFlags(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db
	paveHook = false
	paveAgentsMD = false
	paveAppend = ""

	rootCmd.SetArgs([]string{"pave", "--db", db})
	err := rootCmd.Execute()
	if err == nil {
		t.Fatal("expected error when no flags specified")
	}
	if !strings.Contains(err.Error(), "--hook or --agents-md") {
		t.Errorf("expected guidance about flags, got: %v", err)
	}
}

func TestPaveHookInstall(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	settingsDir := filepath.Join(t.TempDir(), ".claude")
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(settingsDir, "settings.json")

	// Write minimal settings.
	if err := os.WriteFile(settingsPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// We can't easily test the full hook install path because it reads
	// os.UserHomeDir(). Instead, test the source package helpers directly.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	// Verify pave-check command is registered.
	rootCmd.SetArgs([]string{"pave-check", "--help"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("pave-check help: %v", err)
	}
}

func TestPaveCheckFlagCorrection(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	// Seed a flag correction rule: scp -r → -R
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "r", To: "R", Tool: "Bash", Param: "command", Command: "scp", MatchKind: "flag",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	payload := `{"tool_name":"Bash","tool_input":{"command":"scp -r file.txt host:/"}}`
	stdin := strings.NewReader(payload)

	// Capture stdout to check updatedInput.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCheck(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCheck: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if output == "" {
		t.Fatal("expected JSON output with updatedInput, got empty")
	}

	var result hookOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal output: %v\noutput: %s", err, output)
	}

	if result.HookSpecificOutput.PermissionDecision != "allow" {
		t.Errorf("expected permissionDecision=allow, got %q", result.HookSpecificOutput.PermissionDecision)
	}

	corrected, ok := result.HookSpecificOutput.UpdatedInput["command"].(string)
	if !ok {
		t.Fatalf("expected command in updatedInput, got: %v", result.HookSpecificOutput.UpdatedInput)
	}
	if !strings.Contains(corrected, "-R") {
		t.Errorf("expected corrected command to contain -R, got: %s", corrected)
	}
	if strings.Contains(corrected, "-r") {
		t.Errorf("expected -r to be replaced, got: %s", corrected)
	}
}

func TestPaveCheckCombinedFlagCorrection(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "r", To: "R", Tool: "Bash", Param: "command", Command: "scp", MatchKind: "flag",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	payload := `{"tool_name":"Bash","tool_input":{"command":"scp -rP 22 file host:/"}}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCheck(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCheck: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result hookOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}

	corrected := result.HookSpecificOutput.UpdatedInput["command"].(string)
	if !strings.Contains(corrected, "-RP") {
		t.Errorf("expected -RP in corrected command, got: %s", corrected)
	}
}

func TestPaveCheckCommandSubstitution(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "grep", To: "rg", Tool: "Bash", Param: "command", Command: "grep", MatchKind: "command",
		Message: "Use ripgrep instead of grep",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	payload := `{"tool_name":"Bash","tool_input":{"command":"grep -rn pattern ."}}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCheck(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCheck: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result hookOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}

	corrected := result.HookSpecificOutput.UpdatedInput["command"].(string)
	if !strings.HasPrefix(corrected, "rg ") {
		t.Errorf("expected command to start with 'rg ', got: %s", corrected)
	}
	if !strings.Contains(result.HookSpecificOutput.AdditionalContext, "Use ripgrep") {
		t.Errorf("expected custom message in context, got: %s", result.HookSpecificOutput.AdditionalContext)
	}
}

func TestPaveCheckPipeScoping(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "grep", To: "rg", Tool: "Bash", Param: "command", Command: "grep", MatchKind: "command",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	// Only grep should be replaced, not cat.
	payload := `{"tool_name":"Bash","tool_input":{"command":"cat file | grep pattern"}}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCheck(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCheck: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result hookOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}

	corrected := result.HookSpecificOutput.UpdatedInput["command"].(string)
	if !strings.Contains(corrected, "cat file") {
		t.Errorf("cat should be unchanged, got: %s", corrected)
	}
	if !strings.Contains(corrected, "rg pattern") {
		t.Errorf("grep should be replaced with rg, got: %s", corrected)
	}
}

func TestPaveCheckRegexRule(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: `curl\s+-k\b`, To: "curl --cacert /etc/ssl/cert.pem",
		Tool: "Bash", Param: "command", MatchKind: "regex",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	payload := `{"tool_name":"Bash","tool_input":{"command":"curl -k https://example.com"}}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCheck(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCheck: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result hookOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}

	corrected := result.HookSpecificOutput.UpdatedInput["command"].(string)
	if !strings.Contains(corrected, "--cacert") {
		t.Errorf("expected --cacert in corrected command, got: %s", corrected)
	}
}

func TestPaveCheckNoRulesMatch(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	// Rule for scp, but we're sending an ls command.
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "r", To: "R", Tool: "Bash", Param: "command", Command: "scp", MatchKind: "flag",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	payload := `{"tool_name":"Bash","tool_input":{"command":"ls -la /tmp"}}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCheck(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCheck: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// No corrections → no output.
	if output != "" {
		t.Errorf("expected no output when no rules match, got: %s", output)
	}
}

func TestPaveCheckLiteralReplace(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "user@old:", To: "user@new:", Tool: "Bash", Param: "command", Command: "scp", MatchKind: "literal",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	payload := `{"tool_name":"Bash","tool_input":{"command":"scp file.txt user@old:/tmp/"}}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCheck(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCheck: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result hookOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}

	corrected := result.HookSpecificOutput.UpdatedInput["command"].(string)
	if !strings.Contains(corrected, "user@new:") {
		t.Errorf("expected user@new: in corrected command, got: %s", corrected)
	}
	if strings.Contains(corrected, "user@old:") {
		t.Errorf("expected user@old: to be replaced, got: %s", corrected)
	}
}

// TestPaveCheckRecipeSimple verifies a simple recipe match replaces the segment.
func TestPaveCheckRecipeSimple(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "gt await-signal", To: "gt mol status", Tool: "Bash", Param: "command", Command: "gt", MatchKind: "recipe",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	payload := `{"tool_name":"Bash","tool_input":{"command":"gt await-signal"}}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCheck(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCheck: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result hookOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}

	corrected := result.HookSpecificOutput.UpdatedInput["command"].(string)
	if corrected != "gt mol status" {
		t.Errorf("expected 'gt mol status', got: %s", corrected)
	}
}

// TestPaveCheckRecipePrefixTrailingArgs verifies trailing args are dropped.
func TestPaveCheckRecipePrefixTrailingArgs(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "gt await-signal", To: "gt mol status", Tool: "Bash", Param: "command", Command: "gt", MatchKind: "recipe",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	payload := `{"tool_name":"Bash","tool_input":{"command":"gt await-signal --verbose"}}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCheck(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCheck: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result hookOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}

	corrected := result.HookSpecificOutput.UpdatedInput["command"].(string)
	if corrected != "gt mol status" {
		t.Errorf("expected 'gt mol status' (trailing args dropped), got: %s", corrected)
	}
}

// TestPaveCheckRecipeWordBoundary verifies --wispy does not match --wisp.
func TestPaveCheckRecipeWordBoundary(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "bd list --wisp", To: "bd list | grep -i wisp", Tool: "Bash", Param: "command", Command: "bd", MatchKind: "recipe",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	// "bd list --wispy" should NOT match the recipe for "bd list --wisp"
	payload := `{"tool_name":"Bash","tool_input":{"command":"bd list --wispy"}}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCheck(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCheck: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if output != "" {
		t.Errorf("expected no output (passthrough) for word boundary rejection, got: %s", output)
	}
}

// TestPaveCheckRecipePipeScoping verifies only the matched segment is replaced.
func TestPaveCheckRecipePipeScoping(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "gt await-signal", To: "gt mol status", Tool: "Bash", Param: "command", Command: "gt", MatchKind: "recipe",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	payload := `{"tool_name":"Bash","tool_input":{"command":"echo start && gt await-signal"}}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCheck(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCheck: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result hookOutput
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, buf.String())
	}

	corrected := result.HookSpecificOutput.UpdatedInput["command"].(string)
	if !strings.Contains(corrected, "echo start") {
		t.Errorf("expected 'echo start' preserved, got: %s", corrected)
	}
	if !strings.Contains(corrected, "gt mol status") {
		t.Errorf("expected 'gt mol status' in corrected command, got: %s", corrected)
	}
}

// TestPaveCheckRecipeNoMatch verifies no correction when command doesn't match recipe.
func TestPaveCheckRecipeNoMatch(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "gt await-signal", To: "gt mol status", Tool: "Bash", Param: "command", Command: "gt", MatchKind: "recipe",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db

	// "gt convoy wait" should not match the "gt await-signal" recipe
	payload := `{"tool_name":"Bash","tool_input":{"command":"gt convoy wait"}}`
	stdin := strings.NewReader(payload)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runPaveCheck(stdin)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("runPaveCheck: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if output != "" {
		t.Errorf("expected no output (passthrough) for non-matching command, got: %s", output)
	}
}
