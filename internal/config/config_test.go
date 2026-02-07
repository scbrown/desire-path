package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DBPath != "" || cfg.DefaultSource != "" || cfg.DefaultFormat != "" || len(cfg.KnownTools) != 0 {
		t.Fatalf("expected empty config, got %+v", cfg)
	}
}

func TestSaveAndLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subdir", "config.json")
	cfg := &Config{
		DBPath:        "/custom/path.db",
		DefaultSource: "claude-code",
		KnownTools:    []string{"Read", "Write", "Bash"},
		DefaultFormat: "json",
	}
	if err := cfg.SaveTo(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.DBPath != cfg.DBPath {
		t.Errorf("db_path: got %q, want %q", loaded.DBPath, cfg.DBPath)
	}
	if loaded.DefaultSource != cfg.DefaultSource {
		t.Errorf("default_source: got %q, want %q", loaded.DefaultSource, cfg.DefaultSource)
	}
	if loaded.DefaultFormat != cfg.DefaultFormat {
		t.Errorf("default_format: got %q, want %q", loaded.DefaultFormat, cfg.DefaultFormat)
	}
	if len(loaded.KnownTools) != 3 {
		t.Fatalf("known_tools: got %d items, want 3", len(loaded.KnownTools))
	}
	for i, want := range []string{"Read", "Write", "Bash"} {
		if loaded.KnownTools[i] != want {
			t.Errorf("known_tools[%d]: got %q, want %q", i, loaded.KnownTools[i], want)
		}
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadFrom(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGetSet(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{"db_path", "db_path", "/tmp/test.db", "/tmp/test.db"},
		{"default_source", "default_source", "claude-code", "claude-code"},
		{"default_format table", "default_format", "table", "table"},
		{"default_format json", "default_format", "json", "json"},
		{"known_tools", "known_tools", "Read,Write,Bash", "Read,Write,Bash"},
		{"known_tools empty", "known_tools", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			if err := cfg.Set(tt.key, tt.value); err != nil {
				t.Fatalf("set: %v", err)
			}
			got, err := cfg.Get(tt.key)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetUnknownKey(t *testing.T) {
	cfg := &Config{}
	_, err := cfg.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestSetUnknownKey(t *testing.T) {
	cfg := &Config{}
	err := cfg.Set("nonexistent", "value")
	if err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestSetInvalidFormat(t *testing.T) {
	cfg := &Config{}
	err := cfg.Set("default_format", "xml")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}

func TestValidKeys(t *testing.T) {
	keys := ValidKeys()
	if len(keys) != 4 {
		t.Fatalf("expected 4 keys, got %d", len(keys))
	}
	// Verify sorted order.
	for i := 1; i < len(keys); i++ {
		if keys[i] < keys[i-1] {
			t.Errorf("keys not sorted: %q before %q", keys[i-1], keys[i])
		}
	}
}
