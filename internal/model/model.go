// Package model defines core types for desire-path: desires (failed tool calls),
// paths (aggregated patterns), and aliases (tool name mappings).
package model

import (
	"encoding/json"
	"time"
)

// Desire categories classify the nature of a failed tool call.
const (
	// CategoryEnvNeed indicates a missing command or tool that could be installed.
	CategoryEnvNeed = "env-need"
)

// Desire represents a single failed tool call from an AI coding assistant.
type Desire struct {
	ID        string          `json:"id"`
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
	Error     string          `json:"error"`
	Category  string          `json:"category,omitempty"`
	Source    string          `json:"source,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	CWD       string          `json:"cwd,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

// Path represents an aggregated pattern of repeated desires.
type Path struct {
	ID        string    `json:"id"`
	Pattern   string    `json:"pattern"`
	Count     int       `json:"count"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	AliasTo   string    `json:"alias_to,omitempty"`
}

// Alias maps a hallucinated tool name to a real tool, or defines a parameter
// correction rule scoped to a specific tool and parameter.
//
// When Tool and Param are empty, this is a tool-name alias (original behavior):
// the From pattern matches against the tool_name field and blocks with exit 2.
//
// When Tool and Param are set, this is a parameter correction rule: it matches
// against a specific parameter value within calls to that tool.
type Alias struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	Tool      string    `json:"tool,omitempty"`       // target tool ("" = tool-name alias)
	Param     string    `json:"param,omitempty"`      // target parameter
	Command   string    `json:"command,omitempty"`    // target CLI command (e.g., "scp")
	MatchKind string    `json:"match_kind,omitempty"` // "flag", "literal", "command", "regex"
	Message   string    `json:"message,omitempty"`    // custom explanation
	CreatedAt time.Time `json:"created_at"`
}

// IsToolNameAlias returns true if this alias is a simple tool-name mapping.
func (a Alias) IsToolNameAlias() bool {
	return a.Tool == "" && a.Param == ""
}

// Invocation represents a single tool invocation from any source plugin.
type Invocation struct {
	ID         string          `json:"id"`
	Source     string          `json:"source"`
	InstanceID string          `json:"instance_id,omitempty"`
	HostID     string          `json:"host_id,omitempty"`
	ToolName   string          `json:"tool_name"`
	IsError    bool            `json:"is_error"`
	Error      string          `json:"error,omitempty"`
	CWD        string          `json:"cwd,omitempty"`
	Timestamp  time.Time       `json:"timestamp"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}
