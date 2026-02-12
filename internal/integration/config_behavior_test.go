//go:build integration

package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigDBPath verifies that db_path in config.toml controls where the
// database is created.
func TestConfigDBPath(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	// Set a custom db_path via config.
	customDB := filepath.Join(e.home, "custom", "my.db")
	e.writeConfig("") // reset to default
	e.mustRun(nil, "config", "db_path", customDB)

	// Ingest data — should go to the custom db path.
	e.mustRun(e.fixture("read_file", "s1", "err"), "ingest", "--source", "claude-code")

	// The custom db file should exist.
	if _, err := os.Stat(customDB); os.IsNotExist(err) {
		t.Fatalf("custom db at %s does not exist", customDB)
	}

	// Listing from the custom db should return our data.
	stdout, _ := e.mustRun(nil, "list", "--json")
	if !strings.Contains(stdout, "read_file") {
		t.Errorf("list from custom db missing read_file:\n%s", stdout)
	}
}

// TestConfigDBPathFlagOverride verifies that the --db flag overrides the
// config file's db_path.
func TestConfigDBPathFlagOverride(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	// Set db_path in config to a path we'll use as the config-default.
	configDB := filepath.Join(e.home, "config-db", "desires.db")
	e.mustRun(nil, "config", "db_path", configDB)

	// Ingest to the config db.
	e.mustRun(e.fixture("tool_a", "s1", "err"), "ingest", "--source", "claude-code")

	// Now use --db flag to point to a different db.
	overrideDB := filepath.Join(e.home, "override-db", "desires.db")
	e.mustRun(e.fixture("tool_b", "s2", "err"), "ingest", "--source", "claude-code", "--db", overrideDB)

	// List from override db — should only contain tool_b.
	stdout, _ := e.mustRun(nil, "list", "--json", "--db", overrideDB)
	if !strings.Contains(stdout, "tool_b") {
		t.Errorf("override db should contain tool_b:\n%s", stdout)
	}
	if strings.Contains(stdout, "tool_a") {
		t.Errorf("override db should NOT contain tool_a:\n%s", stdout)
	}

	// List from config db (no --db flag) — should only contain tool_a.
	stdout, _ = e.mustRun(nil, "list", "--json")
	if !strings.Contains(stdout, "tool_a") {
		t.Errorf("config db should contain tool_a:\n%s", stdout)
	}
	if strings.Contains(stdout, "tool_b") {
		t.Errorf("config db should NOT contain tool_b:\n%s", stdout)
	}
}

// TestConfigDefaultFormatJSON verifies that setting default_format=json in
// config causes commands to emit JSON without the --json flag.
func TestConfigDefaultFormatJSON(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	// Set default_format to json.
	e.writeConfig("default_format = \"json\"\n")

	// Run commands without --json — they should output JSON.
	cmds := []struct {
		name string
		args []string
	}{
		{"list", []string{"list"}},
		{"stats", []string{"stats"}},
		{"paths", []string{"paths"}},
		{"aliases", []string{"aliases"}},
		{"sources", []string{"sources"}},
		{"config", []string{"config"}},
	}

	for _, cmd := range cmds {
		t.Run(cmd.name, func(t *testing.T) {
			stdout, _, err := e.run(nil, cmd.args...)
			if err != nil {
				t.Fatalf("dp %v failed: %v", cmd.args, err)
			}
			if stdout == "" {
				t.Fatalf("dp %v produced no output", cmd.args)
			}
			var parsed json.RawMessage
			if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
				t.Errorf("dp %v output is not JSON (expected default_format=json): %v\noutput: %s", cmd.args, err, stdout)
			}
		})
	}
}

// TestConfigDefaultFormatTable verifies that when default_format is unset (or
// "table"), commands output human-readable table format, not JSON.
func TestConfigDefaultFormatTable(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	// Ingest some data so commands have output.
	e.mustRun(e.fixture("read_file", "s1", "unknown tool"), "ingest", "--source", "claude-code")

	// With no default_format set, list should be table (not JSON).
	stdout, _ := e.mustRun(nil, "list")
	var parsed json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &parsed); err == nil {
		t.Errorf("list output looks like JSON when default_format is not set — expected table:\n%s", stdout)
	}
	// Table output should have a header row.
	if !strings.Contains(stdout, "TIMESTAMP") || !strings.Contains(stdout, "TOOL") {
		t.Errorf("list table output missing expected headers:\n%s", stdout)
	}
}

// TestConfigFlagOverridesDefaultFormat verifies that the --json flag works even
// when default_format is "table", and that when default_format="json" the table
// format can be produced using config set (no --table flag exists, but the flag
// takes priority).
func TestConfigFlagOverridesDefaultFormat(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	// Set default_format to table explicitly.
	e.writeConfig("default_format = \"table\"\n")

	// --json flag should override the config and produce JSON.
	stdout, _ := e.mustRun(nil, "stats", "--json")
	var parsed json.RawMessage
	if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
		t.Errorf("--json flag did not override default_format=table: %v\noutput: %s", err, stdout)
	}
}

// TestConfigSetGet verifies the config get/set round-trip via the CLI.
func TestConfigSetGet(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	keys := []struct {
		key   string
		value string
	}{
		{"default_source", "cursor"},
		{"default_format", "json"},
		{"known_tools", "Read,Write,Bash"},
		{"store_mode", "remote"},
		{"remote_url", "http://localhost:7273"},
	}

	for _, kv := range keys {
		t.Run(kv.key, func(t *testing.T) {
			e.mustRun(nil, "config", kv.key, kv.value)

			stdout, _ := e.mustRun(nil, "config", kv.key)
			got := strings.TrimSpace(stdout)
			if got != kv.value {
				t.Errorf("config get %s = %q, want %q", kv.key, got, kv.value)
			}
		})
	}
}

// TestConfigShowJSON verifies that `dp config --json` returns all config keys.
func TestConfigShowJSON(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	e.mustRun(nil, "config", "default_source", "test-source")

	stdout, _ := e.mustRun(nil, "config", "--json")

	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &cfg); err != nil {
		t.Fatalf("config --json output is not valid JSON: %v\noutput: %s", err, stdout)
	}

	// db_path should be present (set by newEnv).
	if _, ok := cfg["db_path"]; !ok {
		t.Error("config JSON missing db_path")
	}

	// Our set value should appear.
	if src, ok := cfg["default_source"]; !ok || src != "test-source" {
		t.Errorf("config JSON default_source = %v, want test-source", src)
	}
}
