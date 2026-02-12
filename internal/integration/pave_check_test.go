//go:build integration

package integration

import (
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// exitCode extracts the exit code from an exec error. Returns 0 if err is nil.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// TestPaveCheckAliasMatch verifies pave-check exits 2 when the tool_name
// matches a configured alias, and prints the redirect message on stderr.
func TestPaveCheckAliasMatch(t *testing.T) {
	e := newEnv(t)

	// Create an alias: read_file -> Read
	e.mustRun(nil, "alias", "read_file", "Read")

	// Feed a hook payload with the aliased tool name.
	payload, _ := json.Marshal(map[string]string{"tool_name": "read_file"})
	_, stderr, err := e.run(payload, "pave-check")

	code := exitCode(err)
	if code != 2 {
		t.Fatalf("expected exit code 2, got %d (err: %v, stderr: %q)", code, err, stderr)
	}
	if !strings.Contains(stderr, "Read") {
		t.Errorf("expected stderr to mention target 'Read', got: %q", stderr)
	}
	if !strings.Contains(stderr, "read_file") {
		t.Errorf("expected stderr to mention source 'read_file', got: %q", stderr)
	}
}

// TestPaveCheckNoAlias verifies pave-check exits 0 when the tool_name has
// no alias configured.
func TestPaveCheckNoAlias(t *testing.T) {
	e := newEnv(t)

	payload, _ := json.Marshal(map[string]string{"tool_name": "Bash"})
	_, stderr, err := e.run(payload, "pave-check")

	code := exitCode(err)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d (err: %v, stderr: %q)", code, err, stderr)
	}
}

// TestPaveCheckBadJSON verifies pave-check exits 0 (allow) when stdin
// contains invalid JSON rather than blocking or crashing.
func TestPaveCheckBadJSON(t *testing.T) {
	e := newEnv(t)

	_, stderr, err := e.run([]byte(`{not json`), "pave-check")

	code := exitCode(err)
	if code != 0 {
		t.Fatalf("expected exit code 0 on bad JSON, got %d (err: %v, stderr: %q)", code, err, stderr)
	}
}

// TestPaveCheckEmptyStdin verifies pave-check exits 0 (allow) when stdin
// is empty (no payload provided).
func TestPaveCheckEmptyStdin(t *testing.T) {
	e := newEnv(t)

	_, stderr, err := e.run([]byte{}, "pave-check")

	code := exitCode(err)
	if code != 0 {
		t.Fatalf("expected exit code 0 on empty stdin, got %d (err: %v, stderr: %q)", code, err, stderr)
	}
}

// TestPaveCheckChainedAlias verifies single-hop alias lookup when an
// alias-of-an-alias chain exists: read_file→Read→SuperRead.
// pave-check read_file should exit 2 mentioning "Read" (not "SuperRead"),
// and pave-check Read should exit 2 mentioning "SuperRead".
func TestPaveCheckChainedAlias(t *testing.T) {
	e := newEnv(t)

	// Build alias chain: read_file → Read → SuperRead.
	e.mustRun(nil, "alias", "read_file", "Read")
	e.mustRun(nil, "alias", "Read", "SuperRead")

	// read_file should resolve one hop to Read, not follow chain to SuperRead.
	payload1, _ := json.Marshal(map[string]string{"tool_name": "read_file"})
	_, stderr, err := e.run(payload1, "pave-check")
	code := exitCode(err)
	if code != 2 {
		t.Fatalf("read_file: expected exit code 2, got %d (stderr: %q)", code, stderr)
	}
	if !strings.Contains(stderr, "Read") {
		t.Errorf("read_file: expected stderr to mention 'Read', got: %q", stderr)
	}
	if strings.Contains(stderr, "SuperRead") {
		t.Errorf("read_file: should NOT mention 'SuperRead' (single-hop), got: %q", stderr)
	}

	// Read should resolve one hop to SuperRead.
	payload2, _ := json.Marshal(map[string]string{"tool_name": "Read"})
	_, stderr, err = e.run(payload2, "pave-check")
	code = exitCode(err)
	if code != 2 {
		t.Fatalf("Read: expected exit code 2, got %d (stderr: %q)", code, stderr)
	}
	if !strings.Contains(stderr, "SuperRead") {
		t.Errorf("Read: expected stderr to mention 'SuperRead', got: %q", stderr)
	}
}
