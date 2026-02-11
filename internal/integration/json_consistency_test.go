//go:build integration

package integration

import (
	"encoding/json"
	"testing"
)

// TestAllCommandsJSON is a table-driven test that verifies every command
// supporting --json produces valid JSON output. Each subtest runs a single
// command invocation and validates the output parses as JSON.
func TestAllCommandsJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(e *dpEnv) // optional pre-test setup
		args  []string
		stdin []byte
	}{
		{
			name: "list",
			args: []string{"list", "--json"},
		},
		{
			name: "stats",
			args: []string{"stats", "--json"},
		},
		{
			name: "stats_invocations",
			args: []string{"stats", "--invocations", "--json"},
		},
		{
			name: "paths",
			args: []string{"paths", "--json"},
		},
		{
			name: "aliases",
			args: []string{"aliases", "--json"},
		},
		{
			name: "sources",
			args: []string{"sources", "--json"},
		},
		{
			name: "config",
			args: []string{"config", "--json"},
		},
		{
			name: "inspect",
			args: []string{"inspect", "nonexistent", "--json"},
		},
		{
			name: "similar",
			args: []string{"similar", "read_file", "--json"},
		},
		{
			name: "ingest",
			args:  []string{"ingest", "--source", "claude-code", "--json"},
			stdin: []byte(`{"tool_name":"Read","session_id":"s1","cwd":"/tmp","error":"unknown tool"}`),
		},
		{
			name: "alias_set",
			args: []string{"alias", "read_file", "Read", "--json"},
		},
		{
			name: "alias_delete",
			setup: func(e *dpEnv) {
				e.mustRun(nil, "alias", "to_delete", "Read")
			},
			args: []string{"alias", "--delete", "to_delete", "--json"},
		},
		{
			name: "list_with_data",
			setup: func(e *dpEnv) {
				e.mustRun(e.fixture("read_file", "s1", "unknown tool"), "ingest", "--source", "claude-code")
			},
			args: []string{"list", "--json"},
		},
		{
			name: "stats_with_data",
			setup: func(e *dpEnv) {
				e.mustRun(e.fixture("read_file", "s1", "unknown tool"), "ingest", "--source", "claude-code")
			},
			args: []string{"stats", "--json"},
		},
		{
			name: "paths_with_data",
			setup: func(e *dpEnv) {
				e.mustRun(e.fixture("read_file", "s1", "unknown tool"), "ingest", "--source", "claude-code")
			},
			args: []string{"paths", "--json"},
		},
		{
			name: "inspect_with_data",
			setup: func(e *dpEnv) {
				e.mustRun(e.fixture("read_file", "s1", "unknown tool"), "ingest", "--source", "claude-code")
			},
			args: []string{"inspect", "read_file", "--json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := newEnv(t)

			if tt.setup != nil {
				tt.setup(e)
			}

			stdout, stderr, err := e.run(tt.stdin, tt.args...)
			if err != nil {
				t.Fatalf("dp %v failed: %v\nstdout: %s\nstderr: %s", tt.args, err, stdout, stderr)
			}

			if stdout == "" {
				t.Fatalf("dp %v produced no stdout", tt.args)
			}

			var parsed json.RawMessage
			if err := json.Unmarshal([]byte(stdout), &parsed); err != nil {
				t.Errorf("dp %v output is not valid JSON: %v\noutput: %s", tt.args, err, stdout)
			}
		})
	}
}

// TestExportJSONL verifies export --format json produces valid JSONL (one JSON
// object per line) rather than a single JSON document.
func TestExportJSONL(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	// Ingest two desires.
	e.mustRun(e.fixture("read_file", "s1", "err1"), "ingest", "--source", "claude-code")
	e.mustRun(e.fixture("write_file", "s2", "err2"), "ingest", "--source", "claude-code")

	stdout, _ := e.mustRun(nil, "export", "--format", "json")

	// Each line should be valid JSON.
	lines := splitNonEmpty(stdout)
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 JSONL lines, got %d\noutput: %s", len(lines), stdout)
	}

	for i, line := range lines {
		var obj json.RawMessage
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("line %d is not valid JSON: %v\nline: %s", i, err, line)
		}
	}
}

// TestExportInvocationsJSONL verifies export --type invocations --format json
// produces valid JSONL.
func TestExportInvocationsJSONL(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	// Ingest to create invocations.
	e.mustRun(e.fixture("read_file", "s1", "err"), "ingest", "--source", "claude-code")
	e.mustRun([]byte(`{"tool_name":"Write","session_id":"s2","cwd":"/tmp"}`), "ingest", "--source", "claude-code")

	stdout, _ := e.mustRun(nil, "export", "--type", "invocations", "--format", "json")

	lines := splitNonEmpty(stdout)
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 JSONL lines, got %d\noutput: %s", len(lines), stdout)
	}
	for i, line := range lines {
		var obj json.RawMessage
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Errorf("line %d is not valid JSON: %v\nline: %s", i, err, line)
		}
	}
}

// splitNonEmpty splits s by newlines and returns non-empty lines.
func splitNonEmpty(s string) []string {
	var lines []string
	for _, line := range splitLines(s) {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// splitLines splits a string into lines.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
