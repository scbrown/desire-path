package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/source"
)

func TestInitCmdRequiresSourceFlag(t *testing.T) {
	oldSource := initSource
	oldClaude := initClaudeCode
	defer func() {
		initSource = oldSource
		initClaudeCode = oldClaude
	}()
	initSource = ""
	initClaudeCode = false

	err := initCmd.RunE(initCmd, nil)
	if err == nil {
		t.Fatal("expected error when no flag is specified")
	}
	if !strings.Contains(err.Error(), "--source NAME") {
		t.Errorf("error should mention --source NAME, got: %v", err)
	}
}

func TestInitCmdClaudeCodeDeprecatedFlag(t *testing.T) {
	oldSource := initSource
	oldClaude := initClaudeCode
	defer func() {
		initSource = oldSource
		initClaudeCode = oldClaude
	}()

	// --claude-code should map to --source claude-code.
	initSource = ""
	initClaudeCode = true

	// This will try to actually install; we just verify it resolves the
	// source name by checking it doesn't return "specify a source" error.
	err := initCmd.RunE(initCmd, nil)
	if err != nil && strings.Contains(err.Error(), "--source NAME") {
		t.Errorf("--claude-code should have set source to claude-code, got: %v", err)
	}
}

func TestInitCmdClaudeCodeConflictsWithDifferentSource(t *testing.T) {
	oldSource := initSource
	oldClaude := initClaudeCode
	defer func() {
		initSource = oldSource
		initClaudeCode = oldClaude
	}()

	initSource = "other-source"
	initClaudeCode = true

	err := initCmd.RunE(initCmd, nil)
	if err == nil {
		t.Fatal("expected error for conflicting flags")
	}
	if !strings.Contains(err.Error(), "conflicts") {
		t.Errorf("error should mention conflict, got: %v", err)
	}
}

func TestRunInitUnknownSource(t *testing.T) {
	err := runInit("nonexistent-source", false, "")
	if err == nil {
		t.Fatal("expected error for unknown source")
	}
	if !strings.Contains(err.Error(), "unknown source") {
		t.Errorf("error should mention unknown source, got: %v", err)
	}
}

func TestRunInitSourceWithoutInstaller(t *testing.T) {
	// Register a source that does NOT implement Installer.
	source.Register(&noInstallerSource{name: "test-no-installer-init"})

	err := runInit("test-no-installer-init", false, "")
	if err == nil {
		t.Fatal("expected error for source without Installer")
	}
	if !strings.Contains(err.Error(), "does not support auto-install") {
		t.Errorf("error should mention auto-install, got: %v", err)
	}
}

func TestRunInitAlreadyInstalled(t *testing.T) {
	source.Register(&fakeInstallerSource{
		name:      "test-already-installed",
		installed: true,
	})

	// Capture stdout to verify the message.
	var buf bytes.Buffer
	oldStdout := initCmd.OutOrStdout()
	_ = oldStdout
	// runInit writes to os.Stdout; redirect it.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runInit("test-already-installed", false, "")
		w.Close()
	}()

	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "hooks already configured for test-already-installed") {
		t.Errorf("expected already-configured message, got: %q", got)
	}
}

func TestRunInitAlreadyInstalledJSON(t *testing.T) {
	source.Register(&fakeInstallerSource{
		name:      "test-already-installed-json",
		installed: true,
	})

	// Enable JSON output.
	oldJSON := jsonOutput
	jsonOutput = true
	defer func() { jsonOutput = oldJSON }()

	var buf bytes.Buffer
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runInit("test-already-installed-json", false, "")
		w.Close()
	}()

	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "already_configured") {
		t.Errorf("expected already_configured in JSON output, got: %q", got)
	}
}

func TestRunInitNotYetInstalled(t *testing.T) {
	source.Register(&fakeInstallerSource{
		name:      "test-not-yet-installed",
		installed: false,
	})

	var buf bytes.Buffer
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	errCh := make(chan error, 1)
	go func() {
		errCh <- runInit("test-not-yet-installed", false, "")
		w.Close()
	}()

	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "integration configured") {
		t.Errorf("expected configured message (not already-configured), got: %q", got)
	}
}

// noInstallerSource is a test Source that does not implement Installer.
type noInstallerSource struct {
	name string
}

func (s *noInstallerSource) Name() string                              { return s.name }
func (s *noInstallerSource) Description() string                       { return "test source without installer" }
func (s *noInstallerSource) Extract(raw []byte) (*source.Fields, error) { return nil, nil }

// fakeInstallerSource is a test Source that implements Installer with
// controllable IsInstalled behavior.
type fakeInstallerSource struct {
	name      string
	installed bool
}

func (s *fakeInstallerSource) Name() string                              { return s.name }
func (s *fakeInstallerSource) Description() string                       { return "fake installer source" }
func (s *fakeInstallerSource) Extract(raw []byte) (*source.Fields, error) { return nil, nil }
func (s *fakeInstallerSource) Install(opts source.InstallOpts) error     { return nil }
func (s *fakeInstallerSource) IsInstalled(configDir string) (bool, error) {
	return s.installed, nil
}
