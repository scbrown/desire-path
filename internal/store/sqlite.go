// Package store provides SQLite-backed persistence for desire-path data.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/scbrown/desire-path/internal/model"

	_ "modernc.org/sqlite"
)

const schemaVersion = 1

// SQLiteStore implements Store using a local SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// New opens (or creates) a SQLite database at dbPath.
// It auto-creates the parent directory (e.g. ~/.dp/) and runs
// schema migrations to ensure the database is up to date.
func New(dbPath string) (*SQLiteStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir %s: %w", dir, err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Single connection for WAL mode simplicity.
	db.SetMaxOpenConns(1)

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// migrate runs schema migrations up to the current version.
func (s *SQLiteStore) migrate() error {
	// Create version table if it doesn't exist.
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create version table: %w", err)
	}

	var ver int
	err := s.db.QueryRow("SELECT version FROM schema_version LIMIT 1").Scan(&ver)
	if err == sql.ErrNoRows {
		ver = 0
	} else if err != nil {
		return fmt.Errorf("read version: %w", err)
	}

	if ver < 1 {
		if err := s.migrateV1(); err != nil {
			return err
		}
	}

	return nil
}

func (s *SQLiteStore) migrateV1() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS desires (
			id         TEXT PRIMARY KEY,
			tool_name  TEXT NOT NULL,
			tool_input TEXT,
			error      TEXT NOT NULL,
			source     TEXT,
			session_id TEXT,
			cwd        TEXT,
			timestamp  TEXT NOT NULL,
			metadata   TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_desires_tool_name ON desires(tool_name)`,
		`CREATE INDEX IF NOT EXISTS idx_desires_source ON desires(source)`,
		`CREATE INDEX IF NOT EXISTS idx_desires_timestamp ON desires(timestamp)`,
		`CREATE TABLE IF NOT EXISTS aliases (
			from_name  TEXT PRIMARY KEY,
			to_name    TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`INSERT OR REPLACE INTO schema_version (version) VALUES (1)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate v1: %w", err)
		}
	}
	return nil
}

// RecordDesire persists a single failed tool call.
func (s *SQLiteStore) RecordDesire(ctx context.Context, d model.Desire) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO desires (id, tool_name, tool_input, error, source, session_id, cwd, timestamp, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID,
		d.ToolName,
		nullableJSON(d.ToolInput),
		d.Error,
		nullableString(d.Source),
		nullableString(d.SessionID),
		nullableString(d.CWD),
		d.Timestamp.UTC().Format(time.RFC3339Nano),
		nullableJSON(d.Metadata),
	)
	if err != nil {
		return fmt.Errorf("insert desire: %w", err)
	}
	return nil
}

// ListDesires returns desires matching the given filter options.
func (s *SQLiteStore) ListDesires(ctx context.Context, opts ListOpts) ([]model.Desire, error) {
	query := "SELECT id, tool_name, tool_input, error, source, session_id, cwd, timestamp, metadata FROM desires WHERE 1=1"
	var args []any

	if !opts.Since.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, opts.Since.UTC().Format(time.RFC3339Nano))
	}
	if opts.Source != "" {
		query += " AND source = ?"
		args = append(args, opts.Source)
	}
	if opts.ToolName != "" {
		query += " AND tool_name = ?"
		args = append(args, opts.ToolName)
	}
	query += " ORDER BY timestamp DESC"
	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list desires: %w", err)
	}
	defer rows.Close()

	var desires []model.Desire
	for rows.Next() {
		var d model.Desire
		var toolInput, source, sessionID, cwd, ts, metadata sql.NullString
		if err := rows.Scan(&d.ID, &d.ToolName, &toolInput, &d.Error, &source, &sessionID, &cwd, &ts, &metadata); err != nil {
			return nil, fmt.Errorf("scan desire: %w", err)
		}
		d.Source = source.String
		d.SessionID = sessionID.String
		d.CWD = cwd.String
		if toolInput.Valid && toolInput.String != "" {
			d.ToolInput = []byte(toolInput.String)
		}
		if metadata.Valid && metadata.String != "" {
			d.Metadata = []byte(metadata.String)
		}
		t, err := time.Parse(time.RFC3339Nano, ts.String)
		if err != nil {
			return nil, fmt.Errorf("parse timestamp %q: %w", ts.String, err)
		}
		d.Timestamp = t
		desires = append(desires, d)
	}
	return desires, rows.Err()
}

// GetPaths returns aggregated desire patterns ranked by frequency.
func (s *SQLiteStore) GetPaths(ctx context.Context, opts PathOpts) ([]model.Path, error) {
	query := `SELECT
		d.tool_name,
		COUNT(*) as cnt,
		MIN(d.timestamp) as first_seen,
		MAX(d.timestamp) as last_seen,
		a.to_name
	FROM desires d
	LEFT JOIN aliases a ON a.from_name = d.tool_name`

	var args []any
	if !opts.Since.IsZero() {
		query += " WHERE d.timestamp >= ?"
		args = append(args, opts.Since.UTC().Format(time.RFC3339Nano))
	}
	query += " GROUP BY d.tool_name ORDER BY cnt DESC"
	if opts.Top > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Top)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("get paths: %w", err)
	}
	defer rows.Close()

	var paths []model.Path
	for rows.Next() {
		var p model.Path
		var firstSeen, lastSeen string
		var aliasTo sql.NullString
		if err := rows.Scan(&p.Pattern, &p.Count, &firstSeen, &lastSeen, &aliasTo); err != nil {
			return nil, fmt.Errorf("scan path: %w", err)
		}
		p.ID = p.Pattern // Use tool_name as ID for aggregated paths.
		p.FirstSeen, _ = time.Parse(time.RFC3339Nano, firstSeen)
		p.LastSeen, _ = time.Parse(time.RFC3339Nano, lastSeen)
		if aliasTo.Valid {
			p.AliasTo = aliasTo.String
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// SetAlias creates or updates a mapping from a hallucinated tool name to a real one.
func (s *SQLiteStore) SetAlias(ctx context.Context, from, to string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO aliases (from_name, to_name, created_at) VALUES (?, ?, ?)`,
		from, to, time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("set alias: %w", err)
	}
	return nil
}

// GetAliases returns all configured tool name aliases.
func (s *SQLiteStore) GetAliases(ctx context.Context) ([]model.Alias, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT from_name, to_name, created_at FROM aliases ORDER BY from_name")
	if err != nil {
		return nil, fmt.Errorf("get aliases: %w", err)
	}
	defer rows.Close()

	var aliases []model.Alias
	for rows.Next() {
		var a model.Alias
		var createdAt string
		if err := rows.Scan(&a.From, &a.To, &createdAt); err != nil {
			return nil, fmt.Errorf("scan alias: %w", err)
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		aliases = append(aliases, a)
	}
	return aliases, rows.Err()
}

// DeleteAlias removes an alias by its from_name. Returns true if an alias was deleted.
func (s *SQLiteStore) DeleteAlias(ctx context.Context, from string) (bool, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM aliases WHERE from_name = ?", from)
	if err != nil {
		return false, fmt.Errorf("delete alias: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return n > 0, nil
}

// Stats returns summary statistics about stored desires.
func (s *SQLiteStore) Stats(ctx context.Context) (Stats, error) {
	var st Stats

	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM desires").Scan(&st.TotalDesires); err != nil {
		return st, fmt.Errorf("count desires: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT tool_name) FROM desires").Scan(&st.UniquePaths); err != nil {
		return st, fmt.Errorf("count unique paths: %w", err)
	}

	// Top sources (top 5).
	srcRows, err := s.db.QueryContext(ctx,
		"SELECT source, COUNT(*) FROM desires WHERE source IS NOT NULL AND source != '' GROUP BY source ORDER BY COUNT(*) DESC LIMIT 5")
	if err != nil {
		return st, fmt.Errorf("count sources: %w", err)
	}
	defer srcRows.Close()

	st.TopSources = make(map[string]int)
	for srcRows.Next() {
		var source string
		var count int
		if err := srcRows.Scan(&source, &count); err != nil {
			return st, fmt.Errorf("scan source: %w", err)
		}
		st.TopSources[source] = count
	}
	if err := srcRows.Err(); err != nil {
		return st, err
	}

	// Top 5 most common desires (tool names).
	toolRows, err := s.db.QueryContext(ctx,
		"SELECT tool_name, COUNT(*) as cnt FROM desires GROUP BY tool_name ORDER BY cnt DESC LIMIT 5")
	if err != nil {
		return st, fmt.Errorf("top desires: %w", err)
	}
	defer toolRows.Close()

	for toolRows.Next() {
		var nc NameCount
		if err := toolRows.Scan(&nc.Name, &nc.Count); err != nil {
			return st, fmt.Errorf("scan top desire: %w", err)
		}
		st.TopDesires = append(st.TopDesires, nc)
	}
	if err := toolRows.Err(); err != nil {
		return st, err
	}

	// Date range.
	if st.TotalDesires > 0 {
		var earliest, latest string
		if err := s.db.QueryRowContext(ctx,
			"SELECT MIN(timestamp), MAX(timestamp) FROM desires").Scan(&earliest, &latest); err != nil {
			return st, fmt.Errorf("date range: %w", err)
		}
		st.Earliest, _ = time.Parse(time.RFC3339Nano, earliest)
		st.Latest, _ = time.Parse(time.RFC3339Nano, latest)
	}

	// Time-window counts.
	now := time.Now().UTC()
	for _, w := range []struct {
		dur time.Duration
		dst *int
	}{
		{24 * time.Hour, &st.Last24h},
		{7 * 24 * time.Hour, &st.Last7d},
		{30 * 24 * time.Hour, &st.Last30d},
	} {
		since := now.Add(-w.dur).Format(time.RFC3339Nano)
		if err := s.db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM desires WHERE timestamp >= ?", since).Scan(w.dst); err != nil {
			return st, fmt.Errorf("count since %v: %w", w.dur, err)
		}
	}

	return st, nil
}

// InspectPath returns detailed inspection data for a specific tool name pattern.
func (s *SQLiteStore) InspectPath(ctx context.Context, opts InspectOpts) (*InspectResult, error) {
	topN := opts.TopN
	if topN <= 0 {
		topN = 5
	}

	// Determine whether to use exact match or LIKE.
	// Only % triggers LIKE mode; underscores are common in tool names.
	hasWildcard := strings.Contains(opts.Pattern, "%")
	matchClause := "tool_name = ?"
	if hasWildcard {
		matchClause = "tool_name LIKE ?"
	}

	// Build WHERE clause with optional Since filter.
	where := "WHERE " + matchClause
	args := []any{opts.Pattern}
	if !opts.Since.IsZero() {
		where += " AND timestamp >= ?"
		args = append(args, opts.Since.UTC().Format(time.RFC3339Nano))
	}

	// Summary: total count, first/last seen.
	var result InspectResult
	result.Pattern = opts.Pattern

	var firstSeen, lastSeen sql.NullString
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*), MIN(timestamp), MAX(timestamp) FROM desires "+where, args...,
	).Scan(&result.Total, &firstSeen, &lastSeen)
	if err != nil {
		return nil, fmt.Errorf("inspect summary: %w", err)
	}

	if result.Total == 0 {
		return &result, nil
	}

	if firstSeen.Valid {
		result.FirstSeen, _ = time.Parse(time.RFC3339Nano, firstSeen.String)
	}
	if lastSeen.Valid {
		result.LastSeen, _ = time.Parse(time.RFC3339Nano, lastSeen.String)
	}

	// Check for alias (only meaningful for exact match).
	if !hasWildcard {
		var aliasTo sql.NullString
		_ = s.db.QueryRowContext(ctx,
			"SELECT to_name FROM aliases WHERE from_name = ?", opts.Pattern,
		).Scan(&aliasTo)
		if aliasTo.Valid {
			result.AliasTo = aliasTo.String
		}
	}

	// Histogram: count by date.
	histRows, err := s.db.QueryContext(ctx,
		"SELECT substr(timestamp, 1, 10) AS day, COUNT(*) AS cnt FROM desires "+
			where+" GROUP BY day ORDER BY day", args...)
	if err != nil {
		return nil, fmt.Errorf("inspect histogram: %w", err)
	}
	defer histRows.Close()

	for histRows.Next() {
		var dc DateCount
		if err := histRows.Scan(&dc.Date, &dc.Count); err != nil {
			return nil, fmt.Errorf("scan histogram: %w", err)
		}
		result.Histogram = append(result.Histogram, dc)
	}
	if err := histRows.Err(); err != nil {
		return nil, err
	}

	// Top tool_input values.
	inputRows, err := s.db.QueryContext(ctx,
		fmt.Sprintf("SELECT tool_input, COUNT(*) AS cnt FROM desires %s AND tool_input IS NOT NULL AND tool_input != '' GROUP BY tool_input ORDER BY cnt DESC LIMIT %d", where, topN),
		args...)
	if err != nil {
		return nil, fmt.Errorf("inspect top inputs: %w", err)
	}
	defer inputRows.Close()

	for inputRows.Next() {
		var nc NameCount
		if err := inputRows.Scan(&nc.Name, &nc.Count); err != nil {
			return nil, fmt.Errorf("scan top input: %w", err)
		}
		result.TopInputs = append(result.TopInputs, nc)
	}
	if err := inputRows.Err(); err != nil {
		return nil, err
	}

	// Top error messages.
	errRows, err := s.db.QueryContext(ctx,
		fmt.Sprintf("SELECT error, COUNT(*) AS cnt FROM desires %s AND error != '' GROUP BY error ORDER BY cnt DESC LIMIT %d", where, topN),
		args...)
	if err != nil {
		return nil, fmt.Errorf("inspect top errors: %w", err)
	}
	defer errRows.Close()

	for errRows.Next() {
		var nc NameCount
		if err := errRows.Scan(&nc.Name, &nc.Count); err != nil {
			return nil, fmt.Errorf("scan top error: %w", err)
		}
		result.TopErrors = append(result.TopErrors, nc)
	}
	if err := errRows.Err(); err != nil {
		return nil, err
	}

	return &result, nil
}

// RecordInvocation persists a single tool invocation.
// TODO(dp-pj3): implement with migration v2 invocations table.
func (s *SQLiteStore) RecordInvocation(ctx context.Context, inv model.Invocation) error {
	return fmt.Errorf("invocations table not yet migrated")
}

// ListInvocations returns invocations matching the given filter options.
// TODO(dp-pj3): implement with migration v2 invocations table.
func (s *SQLiteStore) ListInvocations(ctx context.Context, opts InvocationOpts) ([]model.Invocation, error) {
	return nil, fmt.Errorf("invocations table not yet migrated")
}

// InvocationStats returns summary statistics about stored invocations.
// TODO(dp-pj3): implement with migration v2 invocations table.
func (s *SQLiteStore) InvocationStats(ctx context.Context) (InvocationStatsResult, error) {
	return InvocationStatsResult{}, fmt.Errorf("invocations table not yet migrated")
}

// Close releases the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// nullableString returns nil for empty strings, otherwise the string value.
func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullableJSON returns nil for nil/empty JSON, otherwise the string representation.
func nullableJSON(data []byte) any {
	if len(data) == 0 {
		return nil
	}
	return string(data)
}
