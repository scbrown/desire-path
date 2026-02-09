//go:build integration

// Package integration provides end-to-end tests that exercise the compiled dp
// binary. Tests in this package are excluded from normal `go test ./...` runs
// and require the build tag: go test -tags integration ./internal/integration/
//
// TestMain builds the dp binary once into a temporary directory and makes it
// available via dpBin for all tests. Each test creates an isolated dpEnv with
// its own HOME, config, and database so tests can run in parallel.
package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// dpBin holds the path to the compiled dp binary, set once in TestMain.
var dpBin string

// TestMain builds the dp binary and runs all integration tests.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "dp-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmp)

	bin := filepath.Join(tmp, "dp")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/dp")
	// Build from the module root (two levels up from internal/integration).
	cmd.Dir = filepath.Join(modRoot())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "integration: build dp binary: %v\n", err)
		os.Exit(1)
	}

	dpBin = bin
	os.Exit(m.Run())
}

// modRoot returns the module root directory by walking up from this file's
// directory until go.mod is found.
func modRoot() string {
	// Start from the package directory (internal/integration) and walk up.
	dir, err := os.Getwd()
	if err != nil {
		panic(fmt.Sprintf("integration: getwd: %v", err))
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("integration: could not find go.mod in any parent directory")
		}
		dir = parent
	}
}

// dpEnv is an isolated test environment for running dp commands. Each instance
// has its own HOME directory, config file, database, and optional Claude Code
// settings file. Tests should create one via newEnv(t).
type dpEnv struct {
	t       *testing.T
	home    string // isolated HOME directory
	cfgPath string // path to config.toml
	dbPath  string // path to desires.db
}

// newEnv creates an isolated dpEnv for a single test. The environment has its
// own HOME so that dp's default paths (~/.dp/) are sandboxed. The dp config
// is pre-seeded to point at the test database.
func newEnv(t *testing.T) *dpEnv {
	t.Helper()
	home := t.TempDir()

	dpDir := filepath.Join(home, ".dp")
	if err := os.MkdirAll(dpDir, 0o755); err != nil {
		t.Fatalf("create .dp dir: %v", err)
	}

	dbPath := filepath.Join(dpDir, "desires.db")
	cfgPath := filepath.Join(dpDir, "config.toml")

	// Write a minimal config pointing at our isolated database.
	cfg := fmt.Sprintf("db_path = %q\n", dbPath)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	return &dpEnv{
		t:       t,
		home:    home,
		cfgPath: cfgPath,
		dbPath:  dbPath,
	}
}

// run executes `dp <args>` in the test environment and returns stdout, stderr,
// and any error. stdin can be provided as a byte slice (nil for no input).
func (e *dpEnv) run(stdin []byte, args ...string) (stdout, stderr string, err error) {
	e.t.Helper()
	cmd := exec.Command(dpBin, args...)
	cmd.Env = append(os.Environ(),
		"HOME="+e.home,
		"XDG_CONFIG_HOME="+filepath.Join(e.home, ".config"),
	)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// mustRun is like run but calls t.Fatal if the command fails.
func (e *dpEnv) mustRun(stdin []byte, args ...string) (stdout, stderr string) {
	e.t.Helper()
	stdout, stderr, err := e.run(stdin, args...)
	if err != nil {
		e.t.Fatalf("dp %v failed: %v\nstdout: %s\nstderr: %s", args, err, stdout, stderr)
	}
	return stdout, stderr
}

// writeSettings writes a Claude Code settings.json file into the test
// environment at ~/.claude/settings.json.
func (e *dpEnv) writeSettings(content map[string]interface{}) {
	e.t.Helper()
	dir := filepath.Join(e.home, ".claude")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		e.t.Fatalf("create .claude dir: %v", err)
	}
	data, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		e.t.Fatalf("marshal settings: %v", err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		e.t.Fatalf("write settings: %v", err)
	}
}

// writeConfig writes a dp config.toml file with the given content, overwriting
// the default config created by newEnv. The db_path line is always included to
// keep the database sandboxed.
func (e *dpEnv) writeConfig(extra string) {
	e.t.Helper()
	cfg := fmt.Sprintf("db_path = %q\n%s", e.dbPath, extra)
	if err := os.WriteFile(e.cfgPath, []byte(cfg), 0o644); err != nil {
		e.t.Fatalf("write config: %v", err)
	}
}

// fixture returns a Claude Code hook JSON payload for use with dp ingest.
func (e *dpEnv) fixture(toolName, sessionID, errMsg string) []byte {
	e.t.Helper()
	m := map[string]interface{}{
		"tool_name":  toolName,
		"session_id": sessionID,
		"cwd":        e.home,
	}
	if errMsg != "" {
		m["error"] = errMsg
	}
	data, err := json.Marshal(m)
	if err != nil {
		e.t.Fatalf("marshal fixture: %v", err)
	}
	return data
}

// settingsPath returns the path to the Claude Code settings.json in this env.
func (e *dpEnv) settingsPath() string {
	return filepath.Join(e.home, ".claude", "settings.json")
}

// readFile reads a file from the test environment and returns its contents.
func (e *dpEnv) readFile(path string) string {
	e.t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		e.t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
