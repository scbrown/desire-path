package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSourcesCmdTable(t *testing.T) {
	resetFlags(t)

	stdout, _ := captureStdoutAndStderr(t, func() {
		rootCmd.SetArgs([]string{"sources"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	// Should contain header and at least the claude-code source.
	if !strings.Contains(stdout, "NAME") {
		t.Error("expected NAME header in table output")
	}
	if !strings.Contains(stdout, "claude-code") {
		t.Error("expected claude-code in table output")
	}
	if !strings.Contains(stdout, "INSTALLER") {
		t.Error("expected INSTALLER header in table output")
	}
}

func TestSourcesCmdJSON(t *testing.T) {
	resetFlags(t)

	stdout, _ := captureStdoutAndStderr(t, func() {
		rootCmd.SetArgs([]string{"sources", "--json"})
		if err := rootCmd.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	var sources []sourceInfo
	if err := json.Unmarshal([]byte(stdout), &sources); err != nil {
		t.Fatalf("unmarshal JSON output: %v\noutput: %s", err, stdout)
	}
	if len(sources) == 0 {
		t.Fatal("expected at least one source")
	}

	// Find claude-code in the results.
	var found bool
	for _, s := range sources {
		if s.Name == "claude-code" {
			found = true
			if s.Description == "" {
				t.Error("expected non-empty description for claude-code")
			}
			if !s.Installer {
				t.Error("expected claude-code to be an installer")
			}
		}
	}
	if !found {
		t.Error("expected claude-code source in JSON output")
	}
}

func TestSourcesCmdJSONEmptyArray(t *testing.T) {
	// The sources command always returns a non-null array.
	resetFlags(t)

	stdout, _ := captureStdoutAndStderr(t, func() {
		jsonOutput = true
		defer func() { jsonOutput = false }()
		if err := listSources(); err != nil {
			t.Fatalf("listSources: %v", err)
		}
	})

	// Verify valid JSON array.
	var sources []sourceInfo
	if err := json.Unmarshal([]byte(stdout), &sources); err != nil {
		t.Fatalf("unmarshal JSON: %v\noutput: %s", err, stdout)
	}
}
