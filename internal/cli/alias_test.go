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

// resetAliasFlags resets all alias command flags to defaults between tests.
func resetAliasFlags(t *testing.T) {
	t.Helper()
	aliasDelete = false
	aliasCmd_ = ""
	aliasFlag = nil
	aliasReplace = ""
	aliasTool = ""
	aliasParam = ""
	aliasRegex = false
	aliasRecipe = false
	aliasMessage = ""
}

func TestAliasCmdSet(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db
	aliasDelete = false

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"alias", "--db", db, "read_file", "Read"})
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

	if !strings.Contains(output, "Alias set: read_file -> Read") {
		t.Errorf("expected set confirmation, got: %s", output)
	}

	// Verify in database.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	aliases, err := s.GetAliases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}
	if aliases[0].From != "read_file" || aliases[0].To != "Read" {
		t.Errorf("got alias %s->%s, want read_file->Read", aliases[0].From, aliases[0].To)
	}
}

func TestAliasCmdUpsert(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db
	aliasDelete = false

	// Set initial alias.
	rootCmd.SetArgs([]string{"alias", "--db", db, "foo", "bar"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("first set: %v", err)
	}

	// Upsert with new target.
	rootCmd.SetArgs([]string{"alias", "--db", db, "foo", "baz"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	aliases, err := s.GetAliases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias after upsert, got %d", len(aliases))
	}
	if aliases[0].To != "baz" {
		t.Errorf("expected upserted value 'baz', got %q", aliases[0].To)
	}
}

func TestAliasCmdDelete(t *testing.T) {
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
	aliasDelete = true
	defer func() { aliasDelete = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"alias", "--db", db, "--delete", "read_file"})
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

	if !strings.Contains(output, "Alias deleted: read_file") {
		t.Errorf("expected delete confirmation, got: %s", output)
	}

	// Verify removed.
	s2, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	aliases, err := s2.GetAliases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases after delete, got %d", len(aliases))
	}
}

func TestAliasCmdDeleteNotFound(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	aliasDelete = true
	defer func() { aliasDelete = false }()

	rootCmd.SetArgs([]string{"alias", "--db", db, "--delete", "nonexistent"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for deleting nonexistent alias")
	}
}

func TestAliasCmdWrongArgCount(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db
	aliasDelete = false

	// Too few args.
	rootCmd.SetArgs([]string{"alias", "--db", db, "only_one"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error with 1 arg")
	}

	// Too many args.
	rootCmd.SetArgs([]string{"alias", "--db", db, "a", "b", "c"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error with 3 args")
	}
}

func TestAliasCmdDeleteWrongArgCount(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db
	aliasDelete = true
	defer func() { aliasDelete = false }()

	// --delete with 0 args.
	rootCmd.SetArgs([]string{"alias", "--db", db, "--delete"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for --delete with 0 args")
	}

	// --delete with 2 args.
	rootCmd.SetArgs([]string{"alias", "--db", db, "--delete", "a", "b"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for --delete with 2 args")
	}
}

func TestAliasesCmdTable(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")

	// Seed aliases.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{From: "read_file", To: "Read"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{From: "write_file", To: "Write"}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	jsonOutput = false

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"aliases", "--db", db})
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

	if !strings.Contains(output, "FROM") || !strings.Contains(output, "TO") {
		t.Errorf("expected table headers, got: %s", output)
	}
	if !strings.Contains(output, "read_file") || !strings.Contains(output, "Read") {
		t.Errorf("expected read_file->Read alias, got: %s", output)
	}
	if !strings.Contains(output, "write_file") || !strings.Contains(output, "Write") {
		t.Errorf("expected write_file->Write alias, got: %s", output)
	}
}

func TestAliasesCmdEmpty(t *testing.T) {
	db := filepath.Join(t.TempDir(), "test.db")
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	jsonOutput = false

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	rootCmd.SetArgs([]string{"aliases", "--db", db})
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

func TestAliasesCmdJSON(t *testing.T) {
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
	defer func() { jsonOutput = false }()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"aliases", "--db", db, "--json"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var aliases []model.Alias
	if err := json.Unmarshal(buf.Bytes(), &aliases); err != nil {
		t.Fatalf("json unmarshal: %v\nOutput: %s", err, buf.String())
	}
	if len(aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(aliases))
	}
	if aliases[0].From != "read_file" || aliases[0].To != "Read" {
		t.Errorf("got %s->%s, want read_file->Read", aliases[0].From, aliases[0].To)
	}
}

func TestAliasCmdFlagCorrection(t *testing.T) {
	resetAliasFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"alias", "--db", db, "--cmd", "scp", "--flag", "r,R"})
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

	if !strings.Contains(output, "Rule set:") {
		t.Errorf("expected rule set confirmation, got: %s", output)
	}

	// Verify in database.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	alias, err := s.GetAlias(context.Background(), "r", "Bash", "command", "scp", "flag")
	if err != nil {
		t.Fatal(err)
	}
	if alias == nil {
		t.Fatal("expected alias, got nil")
	}
	if alias.From != "r" || alias.To != "R" {
		t.Errorf("got %s->%s, want r->R", alias.From, alias.To)
	}
	if alias.Command != "scp" {
		t.Errorf("got command %q, want scp", alias.Command)
	}
	if alias.MatchKind != "flag" {
		t.Errorf("got match_kind %q, want flag", alias.MatchKind)
	}
}

func TestAliasCmdReplace(t *testing.T) {
	resetAliasFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"alias", "--db", db, "--cmd", "grep", "--replace", "rg", "--message", "Use ripgrep"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	alias, err := s.GetAlias(context.Background(), "grep", "Bash", "command", "grep", "command")
	if err != nil {
		t.Fatal(err)
	}
	if alias == nil {
		t.Fatal("expected alias, got nil")
	}
	if alias.To != "rg" {
		t.Errorf("got to=%q, want rg", alias.To)
	}
	if alias.Message != "Use ripgrep" {
		t.Errorf("got message=%q, want 'Use ripgrep'", alias.Message)
	}
}

func TestAliasCmdLiteral(t *testing.T) {
	resetAliasFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db

	rootCmd.SetArgs([]string{"alias", "--db", db, "--cmd", "scp", "user@host:", "user@newhost:"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	alias, err := s.GetAlias(context.Background(), "user@host:", "Bash", "command", "scp", "literal")
	if err != nil {
		t.Fatal(err)
	}
	if alias == nil {
		t.Fatal("expected alias, got nil")
	}
	if alias.To != "user@newhost:" {
		t.Errorf("got to=%q, want user@newhost:", alias.To)
	}
}

func TestAliasCmdToolParam(t *testing.T) {
	resetAliasFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db

	rootCmd.SetArgs([]string{"alias", "--db", db, "--tool", "MyMCP", "--param", "input_path", "/old", "/new"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	alias, err := s.GetAlias(context.Background(), "/old", "MyMCP", "input_path", "", "literal")
	if err != nil {
		t.Fatal(err)
	}
	if alias == nil {
		t.Fatal("expected alias, got nil")
	}
	if alias.To != "/new" {
		t.Errorf("got to=%q, want /new", alias.To)
	}
}

func TestAliasCmdToolParamRegex(t *testing.T) {
	resetAliasFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db

	rootCmd.SetArgs([]string{"alias", "--db", db, "--tool", "Bash", "--param", "command", "--regex", "curl -k", "curl --cacert cert.pem"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	alias, err := s.GetAlias(context.Background(), "curl -k", "Bash", "command", "", "regex")
	if err != nil {
		t.Fatal(err)
	}
	if alias == nil {
		t.Fatal("expected alias, got nil")
	}
	if alias.To != "curl --cacert cert.pem" {
		t.Errorf("got to=%q, want 'curl --cacert cert.pem'", alias.To)
	}
}

func TestAliasCmdDeleteFlag(t *testing.T) {
	resetAliasFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db

	// Seed a flag rule.
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

	rootCmd.SetArgs([]string{"alias", "--db", db, "--delete", "--cmd", "scp", "--flag", "r,R"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("delete: %v", err)
	}

	s2, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	aliases, err := s2.GetAliases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases after delete, got %d", len(aliases))
	}
}

func TestAliasCmdValidation(t *testing.T) {
	resetAliasFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "cmd and tool mutual exclusion",
			args: []string{"alias", "--db", db, "--cmd", "scp", "--tool", "Bash", "--param", "command", "a", "b"},
			want: "mutually exclusive",
		},
		{
			name: "flag without cmd",
			args: []string{"alias", "--db", db, "--flag", "r,R"},
			want: "--flag requires --cmd",
		},
		{
			name: "replace without cmd",
			args: []string{"alias", "--db", db, "--replace", "rg"},
			want: "--replace requires --cmd",
		},
		{
			name: "regex without tool",
			args: []string{"alias", "--db", db, "--regex", "a", "b"},
			want: "--regex requires --tool",
		},
		{
			name: "tool without param",
			args: []string{"alias", "--db", db, "--tool", "Bash", "a", "b"},
			want: "--tool and --param must be used together",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetAliasFlags(t)
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q should contain %q", err.Error(), tt.want)
			}
		})
	}
}

func TestAliasCmdRecipeCreate(t *testing.T) {
	resetAliasFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"alias", "--db", db, "--recipe", "gt await-signal", "while true; do\n  sleep 5\ndone"})
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

	if !strings.Contains(output, "Rule set:") {
		t.Errorf("expected rule set confirmation, got: %s", output)
	}

	// Verify in database.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	alias, err := s.GetAlias(context.Background(), "gt await-signal", "Bash", "command", "gt", "recipe")
	if err != nil {
		t.Fatal(err)
	}
	if alias == nil {
		t.Fatal("expected alias, got nil")
	}
	if alias.To != "while true; do\n  sleep 5\ndone" {
		t.Errorf("got to=%q, want multi-line script", alias.To)
	}
	if alias.Command != "gt" {
		t.Errorf("got command=%q, want gt", alias.Command)
	}
	if alias.MatchKind != "recipe" {
		t.Errorf("got match_kind=%q, want recipe", alias.MatchKind)
	}
}

func TestAliasCmdRecipeWithMessage(t *testing.T) {
	resetAliasFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db

	rootCmd.SetArgs([]string{"alias", "--db", db, "--recipe", "bd list --wisp", "bd list | grep -i wisp", "--message", "bd has no --wisp flag"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	alias, err := s.GetAlias(context.Background(), "bd list --wisp", "Bash", "command", "bd", "recipe")
	if err != nil {
		t.Fatal(err)
	}
	if alias == nil {
		t.Fatal("expected alias, got nil")
	}
	if alias.To != "bd list | grep -i wisp" {
		t.Errorf("got to=%q, want 'bd list | grep -i wisp'", alias.To)
	}
	if alias.Message != "bd has no --wisp flag" {
		t.Errorf("got message=%q, want 'bd has no --wisp flag'", alias.Message)
	}
}

func TestAliasCmdRecipeDelete(t *testing.T) {
	resetAliasFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db

	// Seed a recipe.
	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "gt await-signal", To: "while true; do sleep 5; done",
		Tool: "Bash", Param: "command", Command: "gt", MatchKind: "recipe",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	rootCmd.SetArgs([]string{"alias", "--db", db, "--delete", "--recipe", "gt await-signal"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("delete: %v", err)
	}

	s2, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	aliases, err := s2.GetAliases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(aliases) != 0 {
		t.Errorf("expected 0 aliases after delete, got %d", len(aliases))
	}
}

func TestAliasCmdRecipeValidation(t *testing.T) {
	resetAliasFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")
	dbPath = db

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "recipe and cmd mutual exclusion",
			args: []string{"alias", "--db", db, "--recipe", "--cmd", "scp", "from", "to"},
			want: "--recipe is mutually exclusive",
		},
		{
			name: "recipe and tool mutual exclusion",
			args: []string{"alias", "--db", db, "--recipe", "--tool", "Bash", "--param", "command", "from", "to"},
			want: "--recipe is mutually exclusive",
		},
		{
			name: "recipe and flag mutual exclusion",
			args: []string{"alias", "--db", db, "--recipe", "--cmd", "scp", "--flag", "r,R"},
			want: "--recipe is mutually exclusive",
		},
		{
			name: "recipe and replace mutual exclusion",
			args: []string{"alias", "--db", db, "--recipe", "--cmd", "grep", "--replace", "rg"},
			want: "--recipe is mutually exclusive",
		},
		{
			name: "recipe and regex mutual exclusion",
			args: []string{"alias", "--db", db, "--recipe", "--tool", "Bash", "--param", "command", "--regex", "from", "to"},
			want: "--recipe is mutually exclusive",
		},
		{
			name: "recipe with too few args",
			args: []string{"alias", "--db", db, "--recipe", "only_one"},
			want: "--recipe requires two positional arguments",
		},
		{
			name: "recipe with too many args",
			args: []string{"alias", "--db", db, "--recipe", "a", "b", "c"},
			want: "--recipe requires two positional arguments",
		},
		{
			name: "recipe delete with wrong arg count",
			args: []string{"alias", "--db", db, "--delete", "--recipe"},
			want: "--delete --recipe requires one positional arg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetAliasFlags(t)
			rootCmd.SetArgs(tt.args)
			err := rootCmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error %q should contain %q", err.Error(), tt.want)
			}
		})
	}
}

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"gt await-signal", "gt"},
		{"bd list --wisp", "bd"},
		{"singleword", "singleword"},
		{"gt\tawait-signal", "gt"},
	}
	for _, tt := range tests {
		got := extractCommand(tt.input)
		if got != tt.want {
			t.Errorf("extractCommand(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncateTo(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 40, "short"},
		{"while true; do\n  sleep 5\ndone", 40, "while true; do   sleep 5 done"},
		{"this is a very long string that exceeds the limit by quite a bit", 20, "this is a very lo..."},
		{"newlines\nget\ncollapsed", 40, "newlines get collapsed"},
	}
	for _, tt := range tests {
		got := truncateTo(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncateTo(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

func TestAliasesListWithRules(t *testing.T) {
	resetAliasFlags(t)
	db := filepath.Join(t.TempDir(), "test.db")

	s, err := store.New(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{From: "read_file", To: "Read"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetAlias(context.Background(), model.Alias{
		From: "r", To: "R", Tool: "Bash", Param: "command", Command: "scp", MatchKind: "flag",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	dbPath = db
	jsonOutput = false

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"aliases", "--db", db})
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

	if !strings.Contains(output, "TYPE") {
		t.Errorf("expected TYPE column header, got: %s", output)
	}
	if !strings.Contains(output, "alias") {
		t.Errorf("expected 'alias' type, got: %s", output)
	}
	if !strings.Contains(output, "flag") {
		t.Errorf("expected 'flag' type, got: %s", output)
	}
	if !strings.Contains(output, "scp") {
		t.Errorf("expected 'scp' command, got: %s", output)
	}
}
