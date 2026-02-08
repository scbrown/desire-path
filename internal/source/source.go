// Package source defines the Source plugin interface and a registry for
// source plugins that extract structured fields from raw tool call data.
package source

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// Source extracts structured fields from raw tool call payloads.
// Each source plugin handles a specific AI tool's output format
// (e.g., Claude Code, Cursor).
type Source interface {
	// Name returns the unique identifier for this source (e.g., "claude-code").
	Name() string

	// Extract parses raw bytes and returns universal fields.
	// Source-specific fields are placed in Fields.Extra.
	Extract(raw []byte) (*Fields, error)
}

// Fields holds the universal fields extracted from any source.
// Only fields common to all sources live here; source-specific
// fields go into the Extra map.
type Fields struct {
	// ToolName is the name of the AI tool that was invoked (required).
	ToolName string `json:"tool_name"`
	// InstanceID is an optional session or invocation identifier.
	InstanceID string `json:"instance_id,omitempty"`
	// ToolInput is the raw JSON input passed to the tool (optional).
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
	// CWD is the working directory at the time of the tool call (optional).
	CWD string `json:"cwd,omitempty"`
	// Error is the error message if the tool call failed (optional).
	Error string `json:"error,omitempty"`
	// Extra holds source-specific fields not mapped to universal fields.
	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// Installer is an optional interface that source plugins can implement
// to provide setup and integration support (e.g., configuring hooks
// for dp init).
type Installer interface {
	// Install configures the source integration at the given settings path.
	Install(settingsPath string) error
}

var (
	mu       sync.RWMutex
	registry = make(map[string]Source)
)

// Register adds a source plugin to the registry. It panics if a source
// with the same name is already registered.
func Register(s Source) {
	mu.Lock()
	defer mu.Unlock()
	name := s.Name()
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("source: duplicate registration for %q", name))
	}
	registry[name] = s
}

// Get returns the source plugin with the given name, or nil if not found.
func Get(name string) Source {
	mu.RLock()
	defer mu.RUnlock()
	return registry[name]
}

// Names returns the sorted names of all registered source plugins.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
