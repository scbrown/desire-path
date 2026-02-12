// Package store defines the storage interface for desire-path data.
package store

import (
	"context"
	"time"

	"github.com/scbrown/desire-path/internal/model"
)

// Store is the persistence interface for desires, paths, and aliases.
type Store interface {
	// RecordDesire persists a single failed tool call.
	RecordDesire(ctx context.Context, d model.Desire) error

	// ListDesires returns desires matching the given filter options.
	ListDesires(ctx context.Context, opts ListOpts) ([]model.Desire, error)

	// GetPaths returns aggregated desire patterns ranked by frequency.
	GetPaths(ctx context.Context, opts PathOpts) ([]model.Path, error)

	// SetAlias creates or updates an alias or parameter correction rule.
	SetAlias(ctx context.Context, alias model.Alias) error

	// GetAlias returns a single alias by its composite key, or nil if not found.
	// For tool-name aliases, pass empty strings for tool, param, command, matchKind.
	GetAlias(ctx context.Context, from, tool, param, command, matchKind string) (*model.Alias, error)

	// GetAliases returns all configured aliases and parameter correction rules.
	GetAliases(ctx context.Context) ([]model.Alias, error)

	// DeleteAlias removes an alias by its composite key. Returns true if deleted.
	DeleteAlias(ctx context.Context, from, tool, param, command, matchKind string) (bool, error)

	// GetRulesForTool returns all parameter correction rules for a specific tool.
	// Only returns rules where Tool is non-empty (not tool-name aliases).
	GetRulesForTool(ctx context.Context, tool string) ([]model.Alias, error)

	// Stats returns summary statistics about stored desires.
	Stats(ctx context.Context) (Stats, error)

	// InspectPath returns detailed inspection data for a specific tool name pattern.
	InspectPath(ctx context.Context, opts InspectOpts) (*InspectResult, error)

	// RecordInvocation persists a single tool invocation.
	RecordInvocation(ctx context.Context, inv model.Invocation) error

	// ListInvocations returns invocations matching the given filter options.
	ListInvocations(ctx context.Context, opts InvocationOpts) ([]model.Invocation, error)

	// InvocationStats returns summary statistics about stored invocations.
	InvocationStats(ctx context.Context) (InvocationStatsResult, error)

	// ListTurns returns turns (grouped invocations) matching the given options.
	ListTurns(ctx context.Context, opts TurnOpts) ([]TurnRow, error)

	// TurnPatternStats returns aggregated turn patterns with their counts.
	TurnPatternStats(ctx context.Context, opts TurnOpts) ([]TurnPattern, error)

	// ToolTurnStats returns per-tool turn statistics for the paths --turns view.
	ToolTurnStats(ctx context.Context, opts TurnOpts) ([]ToolTurnStat, error)

	// Close releases any resources held by the store.
	Close() error
}

// ListOpts controls filtering for ListDesires.
type ListOpts struct {
	Since    time.Time // Only desires after this time.
	Source   string    // Filter by source (e.g., "claude-code").
	ToolName string    // Filter by tool name.
	Category string    // Filter by category (e.g., "env-need").
	Limit    int       // Maximum results; 0 means no limit.
}

// PathOpts controls filtering for GetPaths.
type PathOpts struct {
	Top   int       // Maximum paths to return; 0 means no limit.
	Since time.Time // Only aggregate desires after this time.
}

// NameCount pairs a name (tool name or source) with its occurrence count.
type NameCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// InspectOpts controls filtering for InspectPath.
type InspectOpts struct {
	Pattern string    // Tool name pattern (exact match, or SQL LIKE with % wildcards).
	Since   time.Time // Only desires after this time.
	TopN    int       // Maximum number of top inputs/errors to return; 0 defaults to 5.
}

// InspectResult holds detailed inspection data for a tool name pattern.
type InspectResult struct {
	Pattern   string      `json:"pattern"`
	Total     int         `json:"total"`
	FirstSeen time.Time   `json:"first_seen"`
	LastSeen  time.Time   `json:"last_seen"`
	AliasTo   string      `json:"alias_to,omitempty"`
	Histogram []DateCount `json:"histogram"`
	TopInputs []NameCount `json:"top_inputs"`
	TopErrors []NameCount `json:"top_errors"`
}

// DateCount pairs a date string with a count for histogram display.
type DateCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

// Stats holds summary statistics about stored desires.
type Stats struct {
	TotalDesires int            `json:"total_desires"`
	UniquePaths  int            `json:"unique_paths"`
	TopSources   map[string]int `json:"top_sources"`
	TopDesires   []NameCount    `json:"top_desires"`
	Earliest     time.Time      `json:"earliest"`
	Latest       time.Time      `json:"latest"`
	Last24h      int            `json:"last_24h"`
	Last7d       int            `json:"last_7d"`
	Last30d      int            `json:"last_30d"`
}

// InvocationOpts controls filtering for ListInvocations.
type InvocationOpts struct {
	Since      time.Time // Only invocations after this time.
	Source     string    // Filter by source plugin name.
	InstanceID string    // Filter by instance ID.
	ToolName   string    // Filter by tool name.
	ErrorsOnly bool      // Only return invocations with errors.
	Limit      int       // Maximum results; 0 means no limit.
}

// InvocationStatsResult holds summary statistics about stored invocations.
type InvocationStatsResult struct {
	Total       int            `json:"total"`
	UniqueTools int            `json:"unique_tools"`
	TopSources  []NameCount    `json:"top_sources"`
	TopTools    []NameCount    `json:"top_tools"`
	Last24h     int            `json:"last_24h"`
	Last7d      int            `json:"last_7d"`
	Last30d     int            `json:"last_30d"`
	Earliest    time.Time      `json:"earliest"`
	Latest      time.Time      `json:"latest"`
}

// TurnOpts controls filtering for turn-related queries.
type TurnOpts struct {
	MinLength int       // Minimum turn length (tool call count).
	Since     time.Time // Only turns after this time.
	SessionID string    // Filter by session ID.
	Pattern   string    // Filter by abstract pattern (e.g. "Grep → Read{2+} → Edit").
	Limit     int       // Maximum results; 0 means no limit.
}

// TurnRow represents a single turn with its tool sequence.
type TurnRow struct {
	TurnID    string `json:"turn_id"`
	SessionID string `json:"session_id"`
	Length    int    `json:"length"`
	Tools     string `json:"tools"`
}

// TurnPattern represents an abstract tool sequence pattern with frequency.
type TurnPattern struct {
	Pattern     string  `json:"pattern"`
	Count       int     `json:"count"`
	AvgLength   float64 `json:"avg_length"`
	TotalLength int     `json:"-"`
	Sessions    int     `json:"sessions"`
	sessions    map[string]bool
}

// ToolTurnStat holds per-tool turn statistics.
type ToolTurnStat struct {
	ToolName    string  `json:"tool_name"`
	Count       int     `json:"count"`
	AvgTurnLen  float64 `json:"avg_turn_len"`
	LongCount   int     `json:"long_count"`
	LongTurnPct float64 `json:"long_turn_pct"`
}
