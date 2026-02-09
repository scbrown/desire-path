// Package config handles reading and writing the dp configuration file (~/.dp/config.toml).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// Config holds dp configuration settings.
type Config struct {
	DBPath        string   `toml:"db_path,omitempty" json:"db_path,omitempty"`
	DefaultSource string   `toml:"default_source,omitempty" json:"default_source,omitempty"`
	KnownTools    []string `toml:"known_tools,omitempty" json:"known_tools,omitempty"`
	TrackTools    []string `toml:"track_tools,omitempty" json:"track_tools,omitempty"`
	DefaultFormat string   `toml:"default_format,omitempty" json:"default_format,omitempty"`
	StoreMode     string   `toml:"store_mode,omitempty" json:"store_mode,omitempty"`
	RemoteURL     string   `toml:"remote_url,omitempty" json:"remote_url,omitempty"`
}

// validKeys lists the allowed configuration keys.
var validKeys = map[string]bool{
	"db_path":        true,
	"default_source": true,
	"known_tools":    true,
	"track_tools":    true,
	"default_format": true,
	"store_mode":     true,
	"remote_url":     true,
}

// ValidKeys returns the sorted list of valid configuration keys.
func ValidKeys() []string {
	return []string{"db_path", "default_format", "default_source", "known_tools", "remote_url", "store_mode", "track_tools"}
}

// Path returns the default config file path (~/.dp/config.toml).
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".dp", "config.toml")
	}
	return filepath.Join(home, ".dp", "config.toml")
}

// jsonPath returns the legacy JSON config path for backward compatibility.
func jsonPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".dp", "config.json")
	}
	return filepath.Join(home, ".dp", "config.json")
}

// Load reads the config from the default path. If the TOML file does not exist
// but a legacy JSON config (~/.dp/config.json) does, it migrates the JSON config
// to TOML automatically.
func Load() (*Config, error) {
	cfg, err := LoadFrom(Path())
	if err != nil {
		return nil, err
	}
	// If the TOML file didn't exist, check for legacy JSON config.
	if cfg.DBPath == "" && cfg.DefaultSource == "" && cfg.DefaultFormat == "" && cfg.StoreMode == "" && cfg.RemoteURL == "" && len(cfg.KnownTools) == 0 && len(cfg.TrackTools) == 0 {
		if _, statErr := os.Stat(Path()); errors.Is(statErr, os.ErrNotExist) {
			legacy := jsonPath()
			if _, legacyErr := os.Stat(legacy); legacyErr == nil {
				cfg, err = loadJSON(legacy)
				if err != nil {
					return nil, err
				}
				// Migrate: write TOML and remove JSON.
				if saveErr := cfg.SaveTo(Path()); saveErr == nil {
					os.Remove(legacy)
				}
				return cfg, nil
			}
		}
	}
	return cfg, nil
}

// LoadFrom reads the config from a specific path. Returns an empty Config if
// the file does not exist. Supports both TOML and JSON formats (detected by
// file extension; defaults to TOML).
func LoadFrom(path string) (*Config, error) {
	if filepath.Ext(path) == ".json" {
		return loadJSON(path)
	}
	return loadTOML(path)
}

// loadTOML reads a TOML config file.
func loadTOML(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// loadJSON reads a JSON config file (for backward compatibility).
func loadJSON(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	return &cfg, nil
}

// Save writes the config to the default path.
func (c *Config) Save() error {
	return c.SaveTo(Path())
}

// SaveTo writes the config to a specific path, creating parent directories as needed.
// Writes TOML format regardless of file extension.
func (c *Config) SaveTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := toml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}
	return nil
}

// Get returns the string value of a configuration key.
func (c *Config) Get(key string) (string, error) {
	if !validKeys[key] {
		return "", fmt.Errorf("unknown config key %q (valid keys: %s)", key, strings.Join(ValidKeys(), ", "))
	}
	switch key {
	case "db_path":
		return c.DBPath, nil
	case "default_source":
		return c.DefaultSource, nil
	case "known_tools":
		return strings.Join(c.KnownTools, ","), nil
	case "track_tools":
		if len(c.TrackTools) == 0 {
			return "", nil
		}
		b, err := json.Marshal(c.TrackTools)
		if err != nil {
			return "", fmt.Errorf("marshaling track_tools: %w", err)
		}
		return string(b), nil
	case "default_format":
		return c.DefaultFormat, nil
	case "store_mode":
		return c.StoreMode, nil
	case "remote_url":
		return c.RemoteURL, nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}

// Set assigns a value to a configuration key.
func (c *Config) Set(key, value string) error {
	if !validKeys[key] {
		return fmt.Errorf("unknown config key %q (valid keys: %s)", key, strings.Join(ValidKeys(), ", "))
	}
	switch key {
	case "db_path":
		c.DBPath = value
	case "default_source":
		c.DefaultSource = value
	case "known_tools":
		if value == "" {
			c.KnownTools = nil
		} else {
			c.KnownTools = strings.Split(value, ",")
		}
	case "track_tools":
		if value == "" {
			c.TrackTools = nil
		} else {
			var tools []string
			if err := json.Unmarshal([]byte(value), &tools); err != nil {
				return fmt.Errorf("track_tools must be a JSON array of strings, e.g. '[\"Read\",\"Bash\"]': %w", err)
			}
			for i, name := range tools {
				if strings.TrimSpace(name) == "" {
					return fmt.Errorf("track_tools[%d]: tool name must be non-empty", i)
				}
			}
			c.TrackTools = tools
		}
	case "default_format":
		if value != "" && value != "table" && value != "json" {
			return fmt.Errorf("default_format must be \"table\" or \"json\", got %q", value)
		}
		c.DefaultFormat = value
	case "store_mode":
		if value != "" && value != "local" && value != "remote" {
			return fmt.Errorf("store_mode must be \"local\" or \"remote\", got %q", value)
		}
		c.StoreMode = value
	case "remote_url":
		c.RemoteURL = value
	}
	return nil
}
