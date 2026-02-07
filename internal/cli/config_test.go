package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/scbrown/desire-path/internal/config"
)

func TestConfigCmdShowEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "config.json")
	jsonOutput = false
	defer func() {
		configPath = config.Path()
		jsonOutput = false
	}()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"config"})
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

	if !strings.Contains(output, "KEY") || !strings.Contains(output, "VALUE") {
		t.Errorf("expected table headers, got: %s", output)
	}
	if !strings.Contains(output, "db_path") {
		t.Errorf("expected db_path key, got: %s", output)
	}
	if !strings.Contains(output, "(not set)") {
		t.Errorf("expected (not set) for empty values, got: %s", output)
	}
}

func TestConfigCmdGet(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	configPath = cfgPath
	jsonOutput = false
	defer func() {
		configPath = config.Path()
		jsonOutput = false
	}()

	// Seed config.
	cfg := &config.Config{DBPath: "/custom/path.db"}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"config", "db_path"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := strings.TrimSpace(buf.String())

	if output != "/custom/path.db" {
		t.Errorf("got %q, want %q", output, "/custom/path.db")
	}
}

func TestConfigCmdGetEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "config.json")
	jsonOutput = false
	defer func() {
		configPath = config.Path()
		jsonOutput = false
	}()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"config", "db_path"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := strings.TrimSpace(buf.String())

	if output != "" {
		t.Errorf("expected empty output for unset key, got %q", output)
	}
}

func TestConfigCmdSet(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	configPath = cfgPath
	jsonOutput = false
	defer func() {
		configPath = config.Path()
		jsonOutput = false
	}()

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"config", "default_source", "claude-code"})
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

	if !strings.Contains(output, "default_source = claude-code") {
		t.Errorf("expected confirmation, got: %s", output)
	}

	// Verify persisted.
	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DefaultSource != "claude-code" {
		t.Errorf("persisted value: got %q, want %q", cfg.DefaultSource, "claude-code")
	}
}

func TestConfigCmdSetKnownTools(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	configPath = cfgPath
	jsonOutput = false
	defer func() {
		configPath = config.Path()
		jsonOutput = false
	}()

	rootCmd.SetArgs([]string{"config", "known_tools", "Read,Write,Bash"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	cfg, err := config.LoadFrom(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.KnownTools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(cfg.KnownTools))
	}
	want := []string{"Read", "Write", "Bash"}
	for i, w := range want {
		if cfg.KnownTools[i] != w {
			t.Errorf("known_tools[%d]: got %q, want %q", i, cfg.KnownTools[i], w)
		}
	}
}

func TestConfigCmdInvalidKey(t *testing.T) {
	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "config.json")
	jsonOutput = false
	defer func() {
		configPath = config.Path()
		jsonOutput = false
	}()

	rootCmd.SetArgs([]string{"config", "bad_key"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestConfigCmdShowJSON(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")
	configPath = cfgPath
	jsonOutput = true
	defer func() {
		configPath = config.Path()
		jsonOutput = false
	}()

	cfg := &config.Config{
		DBPath:        "/custom/path.db",
		DefaultSource: "claude-code",
	}
	if err := cfg.SaveTo(cfgPath); err != nil {
		t.Fatal(err)
	}

	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	rootCmd.SetArgs([]string{"config", "--json"})
	if err := rootCmd.Execute(); err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("execute: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)

	var result config.Config
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("json unmarshal: %v\nOutput: %s", err, buf.String())
	}
	if result.DBPath != "/custom/path.db" {
		t.Errorf("db_path: got %q, want %q", result.DBPath, "/custom/path.db")
	}
	if result.DefaultSource != "claude-code" {
		t.Errorf("default_source: got %q, want %q", result.DefaultSource, "claude-code")
	}
}

func TestConfigCmdTooManyArgs(t *testing.T) {
	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, "config.json")
	jsonOutput = false
	defer func() {
		configPath = config.Path()
		jsonOutput = false
	}()

	rootCmd.SetArgs([]string{"config", "a", "b", "c"})
	if err := rootCmd.Execute(); err == nil {
		t.Fatal("expected error for too many args")
	}
}
