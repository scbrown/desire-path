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

	// SetAlias creates or updates a mapping from a hallucinated tool name to a real one.
	SetAlias(ctx context.Context, from, to string) error

	// GetAliases returns all configured tool name aliases.
	GetAliases(ctx context.Context) ([]model.Alias, error)

	// DeleteAlias removes an alias by its from_name. Returns true if an alias was deleted.
	DeleteAlias(ctx context.Context, from string) (bool, error)

	// Stats returns summary statistics about stored desires.
	Stats(ctx context.Context) (Stats, error)

	// InspectPath returns detailed inspection data for a specific tool name pattern.
	InspectPath(ctx context.Context, opts InspectOpts) (*InspectResult, error)

	// RecordInvocation persists a single tool invocation.
	RecordInvocation(ctx context.Context, inv model.Invocation) error

	// ListInvocations returns invocations matching the given filter options.
	ListInvocations(ctx context.Context, opts InvocationListOpts) ([]model.Invocation, error)

	// InvocationStats returns aggregate statistics about stored invocations.
	InvocationStats(ctx context.Context) (InvocationStatsResult, error)

	// Close releases any resources held by the store.
	Close() error
}

// ListOpts controls filtering for ListDesires.
type ListOpts struct {
	Since    time.Time // Only desires after this time.
	Source   string    // Filter by source (e.g., "claude-code").
	ToolName string    // Filter by tool name.
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

// InvocationListOpts controls filtering for ListInvocations.
type InvocationListOpts struct {
	Since      time.Time // Only invocations after this time.
	Source     string    // Filter by source.
	InstanceID string    // Filter by instance ID.
	ToolName   string    // Filter by tool name.
	IsError    *bool     // Filter by error status; nil means no filter.
	Limit      int       // Maximum results; 0 means no limit.
}

// InvocationStatsResult holds aggregate statistics about stored invocations.
type InvocationStatsResult struct {
	Total      int         `json:"total"`
	Errors     int         `json:"errors"`
	TopTools   []NameCount `json:"top_tools"`
	TopSources []NameCount `json:"top_sources"`
	Earliest   time.Time   `json:"earliest"`
	Latest     time.Time   `json:"latest"`
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
