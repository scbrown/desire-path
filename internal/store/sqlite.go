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

const schemaVersion = 4

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

	if ver < 2 {
		if err := s.migrateV2(); err != nil {
			return err
		}
	}

	if ver < 3 {
		if err := s.migrateV3(); err != nil {
			return err
		}
	}

	if ver < 4 {
		if err := s.migrateV4(); err != nil {
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

func (s *SQLiteStore) migrateV2() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS invocations (
			id          TEXT PRIMARY KEY,
			source      TEXT NOT NULL,
			instance_id TEXT,
			host_id     TEXT,
			tool_name   TEXT NOT NULL,
			is_error    INTEGER NOT NULL DEFAULT 0,
			error       TEXT,
			cwd         TEXT,
			timestamp   TEXT NOT NULL,
			metadata    TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_invocations_source ON invocations(source)`,
		`CREATE INDEX IF NOT EXISTS idx_invocations_instance_id ON invocations(instance_id)`,
		`CREATE INDEX IF NOT EXISTS idx_invocations_tool_name ON invocations(tool_name)`,
		`CREATE INDEX IF NOT EXISTS idx_invocations_timestamp ON invocations(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_invocations_is_error ON invocations(is_error)`,
		`UPDATE schema_version SET version = 2`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate v2: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) migrateV3() error {
	stmts := []string{
		// Recreate aliases table with composite PK and new columns.
		`CREATE TABLE IF NOT EXISTS aliases_v3 (
			from_name  TEXT NOT NULL,
			to_name    TEXT NOT NULL,
			tool       TEXT NOT NULL DEFAULT '',
			param      TEXT NOT NULL DEFAULT '',
			command    TEXT NOT NULL DEFAULT '',
			match_kind TEXT NOT NULL DEFAULT '',
			message    TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			PRIMARY KEY (from_name, tool, param, command, match_kind)
		)`,
		`INSERT OR IGNORE INTO aliases_v3 (from_name, to_name, tool, param, command, match_kind, message, created_at)
			SELECT from_name, to_name, '', '', '', '', '', created_at FROM aliases`,
		`DROP TABLE aliases`,
		`ALTER TABLE aliases_v3 RENAME TO aliases`,
		`CREATE INDEX IF NOT EXISTS idx_aliases_tool_command ON aliases(tool, command)`,
		`UPDATE schema_version SET version = 3`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate v3: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) migrateV4() error {
	stmts := []string{
		`ALTER TABLE desires ADD COLUMN category TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_desires_category ON desires(category)`,
		`UPDATE schema_version SET version = 4`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate v4: %w", err)
		}
	}
	return nil
}

// RecordDesire persists a single failed tool call.
func (s *SQLiteStore) RecordDesire(ctx context.Context, d model.Desire) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO desires (id, tool_name, tool_input, error, category, source, session_id, cwd, timestamp, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID,
		d.ToolName,
		nullableJSON(d.ToolInput),
		d.Error,
		nullableString(d.Category),
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
	query := "SELECT id, tool_name, tool_input, error, category, source, session_id, cwd, timestamp, metadata FROM desires WHERE 1=1"
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
	if opts.Category != "" {
		query += " AND category = ?"
		args = append(args, opts.Category)
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
		var toolInput, category, source, sessionID, cwd, ts, metadata sql.NullString
		if err := rows.Scan(&d.ID, &d.ToolName, &toolInput, &d.Error, &category, &source, &sessionID, &cwd, &ts, &metadata); err != nil {
			return nil, fmt.Errorf("scan desire: %w", err)
		}
		d.Category = category.String
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
	LEFT JOIN aliases a ON a.from_name = d.tool_name AND a.tool = '' AND a.param = ''`

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

// SetAlias creates or updates an alias or parameter correction rule.
func (s *SQLiteStore) SetAlias(ctx context.Context, a model.Alias) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO aliases (from_name, to_name, tool, param, command, match_kind, message, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		a.From, a.To, a.Tool, a.Param, a.Command, a.MatchKind, a.Message,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("set alias: %w", err)
	}
	return nil
}

// GetAlias returns a single alias by its composite key, or nil if not found.
func (s *SQLiteStore) GetAlias(ctx context.Context, from, tool, param, command, matchKind string) (*model.Alias, error) {
	var a model.Alias
	var createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT from_name, to_name, tool, param, command, match_kind, message, created_at
		 FROM aliases WHERE from_name = ? AND tool = ? AND param = ? AND command = ? AND match_kind = ?`,
		from, tool, param, command, matchKind,
	).Scan(&a.From, &a.To, &a.Tool, &a.Param, &a.Command, &a.MatchKind, &a.Message, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get alias: %w", err)
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return &a, nil
}

// GetAliases returns all configured aliases and parameter correction rules.
func (s *SQLiteStore) GetAliases(ctx context.Context) ([]model.Alias, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT from_name, to_name, tool, param, command, match_kind, message, created_at
		 FROM aliases ORDER BY tool, command, param, from_name`)
	if err != nil {
		return nil, fmt.Errorf("get aliases: %w", err)
	}
	defer rows.Close()

	var aliases []model.Alias
	for rows.Next() {
		var a model.Alias
		var createdAt string
		if err := rows.Scan(&a.From, &a.To, &a.Tool, &a.Param, &a.Command, &a.MatchKind, &a.Message, &createdAt); err != nil {
			return nil, fmt.Errorf("scan alias: %w", err)
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		aliases = append(aliases, a)
	}
	return aliases, rows.Err()
}

// DeleteAlias removes an alias by its composite key. Returns true if deleted.
func (s *SQLiteStore) DeleteAlias(ctx context.Context, from, tool, param, command, matchKind string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM aliases WHERE from_name = ? AND tool = ? AND param = ? AND command = ? AND match_kind = ?`,
		from, tool, param, command, matchKind)
	if err != nil {
		return false, fmt.Errorf("delete alias: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return n > 0, nil
}

// GetRulesForTool returns all parameter correction rules for a specific tool.
func (s *SQLiteStore) GetRulesForTool(ctx context.Context, tool string) ([]model.Alias, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT from_name, to_name, tool, param, command, match_kind, message, created_at
		 FROM aliases WHERE tool = ? ORDER BY command, param, from_name`, tool)
	if err != nil {
		return nil, fmt.Errorf("get rules for tool: %w", err)
	}
	defer rows.Close()

	var rules []model.Alias
	for rows.Next() {
		var a model.Alias
		var createdAt string
		if err := rows.Scan(&a.From, &a.To, &a.Tool, &a.Param, &a.Command, &a.MatchKind, &a.Message, &createdAt); err != nil {
			return nil, fmt.Errorf("scan rule: %w", err)
		}
		a.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		rules = append(rules, a)
	}
	return rules, rows.Err()
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
			"SELECT to_name FROM aliases WHERE from_name = ? AND tool = '' AND param = ''", opts.Pattern,
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
func (s *SQLiteStore) RecordInvocation(ctx context.Context, inv model.Invocation) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO invocations (id, source, instance_id, host_id, tool_name, is_error, error, cwd, timestamp, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.ID,
		inv.Source,
		nullableString(inv.InstanceID),
		nullableString(inv.HostID),
		inv.ToolName,
		boolToInt(inv.IsError),
		nullableString(inv.Error),
		nullableString(inv.CWD),
		inv.Timestamp.UTC().Format(time.RFC3339Nano),
		nullableJSON(inv.Metadata),
	)
	if err != nil {
		return fmt.Errorf("insert invocation: %w", err)
	}
	return nil
}

// ListInvocations returns invocations matching the given filter options.
func (s *SQLiteStore) ListInvocations(ctx context.Context, opts InvocationOpts) ([]model.Invocation, error) {
	query := "SELECT id, source, instance_id, host_id, tool_name, is_error, error, cwd, timestamp, metadata FROM invocations WHERE 1=1"
	var args []any

	if !opts.Since.IsZero() {
		query += " AND timestamp >= ?"
		args = append(args, opts.Since.UTC().Format(time.RFC3339Nano))
	}
	if opts.Source != "" {
		query += " AND source = ?"
		args = append(args, opts.Source)
	}
	if opts.InstanceID != "" {
		query += " AND instance_id = ?"
		args = append(args, opts.InstanceID)
	}
	if opts.ToolName != "" {
		query += " AND tool_name = ?"
		args = append(args, opts.ToolName)
	}
	if opts.ErrorsOnly {
		query += " AND is_error = 1"
	}
	query += " ORDER BY timestamp DESC"
	if opts.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", opts.Limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list invocations: %w", err)
	}
	defer rows.Close()

	var invocations []model.Invocation
	for rows.Next() {
		var inv model.Invocation
		var instanceID, hostID, errStr, cwd, ts, metadata sql.NullString
		var isError int
		if err := rows.Scan(&inv.ID, &inv.Source, &instanceID, &hostID, &inv.ToolName, &isError, &errStr, &cwd, &ts, &metadata); err != nil {
			return nil, fmt.Errorf("scan invocation: %w", err)
		}
		inv.InstanceID = instanceID.String
		inv.HostID = hostID.String
		inv.IsError = isError != 0
		inv.Error = errStr.String
		inv.CWD = cwd.String
		if metadata.Valid && metadata.String != "" {
			inv.Metadata = []byte(metadata.String)
		}
		t, err := time.Parse(time.RFC3339Nano, ts.String)
		if err != nil {
			return nil, fmt.Errorf("parse timestamp %q: %w", ts.String, err)
		}
		inv.Timestamp = t
		invocations = append(invocations, inv)
	}
	return invocations, rows.Err()
}

// InvocationStats returns summary statistics about stored invocations.
func (s *SQLiteStore) InvocationStats(ctx context.Context) (InvocationStatsResult, error) {
	var st InvocationStatsResult

	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM invocations").Scan(&st.Total); err != nil {
		return st, fmt.Errorf("count invocations: %w", err)
	}

	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT tool_name) FROM invocations").Scan(&st.UniqueTools); err != nil {
		return st, fmt.Errorf("count unique tools: %w", err)
	}

	// Top sources (top 5).
	srcRows, err := s.db.QueryContext(ctx,
		"SELECT source, COUNT(*) as cnt FROM invocations GROUP BY source ORDER BY cnt DESC LIMIT 5")
	if err != nil {
		return st, fmt.Errorf("top sources: %w", err)
	}
	defer srcRows.Close()

	for srcRows.Next() {
		var nc NameCount
		if err := srcRows.Scan(&nc.Name, &nc.Count); err != nil {
			return st, fmt.Errorf("scan source: %w", err)
		}
		st.TopSources = append(st.TopSources, nc)
	}
	if err := srcRows.Err(); err != nil {
		return st, err
	}

	// Top tools (top 5).
	toolRows, err := s.db.QueryContext(ctx,
		"SELECT tool_name, COUNT(*) as cnt FROM invocations GROUP BY tool_name ORDER BY cnt DESC LIMIT 5")
	if err != nil {
		return st, fmt.Errorf("top tools: %w", err)
	}
	defer toolRows.Close()

	for toolRows.Next() {
		var nc NameCount
		if err := toolRows.Scan(&nc.Name, &nc.Count); err != nil {
			return st, fmt.Errorf("scan tool: %w", err)
		}
		st.TopTools = append(st.TopTools, nc)
	}
	if err := toolRows.Err(); err != nil {
		return st, err
	}

	// Date range.
	if st.Total > 0 {
		var earliest, latest string
		if err := s.db.QueryRowContext(ctx,
			"SELECT MIN(timestamp), MAX(timestamp) FROM invocations").Scan(&earliest, &latest); err != nil {
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
			"SELECT COUNT(*) FROM invocations WHERE timestamp >= ?", since).Scan(w.dst); err != nil {
			return st, fmt.Errorf("count since %v: %w", w.dur, err)
		}
	}

	return st, nil
}

// Close releases the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// boolToInt converts a bool to an integer for SQLite storage.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
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
