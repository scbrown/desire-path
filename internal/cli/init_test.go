package cli

import (
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
	err := runInit("nonexistent-source", false)
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

	err := runInit("test-no-installer-init", false)
	if err == nil {
		t.Fatal("expected error for source without Installer")
	}
	if !strings.Contains(err.Error(), "does not support auto-install") {
		t.Errorf("error should mention auto-install, got: %v", err)
	}
}

// noInstallerSource is a test Source that does not implement Installer.
type noInstallerSource struct {
	name string
}

func (s *noInstallerSource) Name() string                              { return s.name }
func (s *noInstallerSource) Description() string                       { return "test source without installer" }
func (s *noInstallerSource) Extract(raw []byte) (*source.Fields, error) { return nil, nil }
