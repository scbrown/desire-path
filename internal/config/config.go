// Package config handles reading and writing the dp configuration file (~/.dp/config.json).
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Config holds dp configuration settings.
type Config struct {
	DBPath        string   `json:"db_path,omitempty"`
	DefaultSource string   `json:"default_source,omitempty"`
	KnownTools    []string `json:"known_tools,omitempty"`
	DefaultFormat string   `json:"default_format,omitempty"`
}

// validKeys lists the allowed configuration keys.
var validKeys = map[string]bool{
	"db_path":        true,
	"default_source": true,
	"known_tools":    true,
	"default_format": true,
}

// ValidKeys returns the sorted list of valid configuration keys.
func ValidKeys() []string {
	return []string{"db_path", "default_format", "default_source", "known_tools"}
}

// Path returns the default config file path (~/.dp/config.json).
func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".dp", "config.json")
	}
	return filepath.Join(home, ".dp", "config.json")
}

// Load reads the config from the default path. Returns an empty Config if the file does not exist.
func Load() (*Config, error) {
	return LoadFrom(Path())
}

// LoadFrom reads the config from a specific path. Returns an empty Config if the file does not exist.
func LoadFrom(path string) (*Config, error) {
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
func (c *Config) SaveTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	data = append(data, '\n')
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
	case "default_format":
		return c.DefaultFormat, nil
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
	case "default_format":
		if value != "" && value != "table" && value != "json" {
			return fmt.Errorf("default_format must be \"table\" or \"json\", got %q", value)
		}
		c.DefaultFormat = value
	}
	return nil
}
