// Package model defines core types for desire-path: desires (failed tool calls),
// paths (aggregated patterns), and aliases (tool name mappings).
package model

import (
	"encoding/json"
	"time"
)

// Desire represents a single failed tool call from an AI coding assistant.
type Desire struct {
	ID        string          `json:"id"`
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
	Error     string          `json:"error"`
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

// Alias maps a hallucinated tool name to a real tool or command.
type Alias struct {
	From      string    `json:"from"`
	To        string    `json:"to"`
	CreatedAt time.Time `json:"created_at"`
}
