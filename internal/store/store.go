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

	// Stats returns summary statistics about stored desires.
	Stats(ctx context.Context) (Stats, error)

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

// Stats holds summary statistics about stored desires.
type Stats struct {
	TotalDesires int            `json:"total_desires"`
	UniquePaths  int            `json:"unique_paths"`
	TopSources   map[string]int `json:"top_sources"`
}
