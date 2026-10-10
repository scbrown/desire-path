package adoption

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

const (
	windowSize  = 10
	donorLimit  = 1024
	maxSessions = 4096
	maxEvents   = 50000
)

// Store is separate from invocation storage: observation never replaces a
// record, enriches from a transcript, or changes whether a payload is delivered.
type Store struct {
	dir      string
	db       *sql.DB
	key      []byte
	Resolver Resolver
	Draw     func(int64) (int64, error)
}

// Open creates a private, bounded collector store. Every transition acquires
// SQLite's writer lock before reading counters or choosing a donor.
func Open(ctx context.Context, dir string, resolver Resolver) (*Store, error) {
	if !filepath.IsAbs(dir) {
		return nil, errors.New("collector directory must be absolute")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.New("collector directory unavailable")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("collector directory must be private and not a symlink")
	}
	dbPath := filepath.Join(dir, "adoption.sqlite")
	if info, err := os.Lstat(dbPath); err == nil && !info.Mode().IsRegular() {
		return nil, errors.New("collector database must be a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, errors.New("collector database unavailable")
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, errors.New("collector unavailable")
	}
	db.SetMaxOpenConns(1)
	s := &Store{dir: dir, db: db, Resolver: resolver, Draw: func(n int64) (int64, error) {
		v, e := rand.Int(rand.Reader, big.NewInt(n))
		if e != nil {
			return 0, e
		}
		return v.Int64(), nil
	}}
	err = s.transaction(ctx, func(c *sql.Conn) error {
		var tables int
		if err := c.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&tables); err != nil {
			return err
		}
		if tables != 0 {
			var version string
			if err := c.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='schema'`).Scan(&version); err != nil || version != "dp-adoption/1" {
				return errors.New("foreign or unsupported collector schema")
			}
		}
		for _, statement := range []string{
			`CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value BLOB NOT NULL)`,
			`CREATE TABLE IF NOT EXISTS sessions (id TEXT PRIMARY KEY, seq INTEGER NOT NULL)`,
			`CREATE TABLE IF NOT EXISTS calls (session TEXT NOT NULL, id TEXT NOT NULL, seq INTEGER NOT NULL, finished INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(session,id))`,
			`CREATE TABLE IF NOT EXISTS payloads (id TEXT PRIMARY KEY, session TEXT NOT NULL, timestamp_ns INTEGER NOT NULL, boundary INTEGER NOT NULL, paths TEXT NOT NULL, base_paths TEXT NOT NULL, base_id TEXT NOT NULL, status TEXT NOT NULL, touch_seen INTEGER NOT NULL DEFAULT 0, base_touch_seen INTEGER NOT NULL DEFAULT 0, adopted INTEGER, base_adopted INTEGER, uncertain INTEGER NOT NULL DEFAULT 0, completed INTEGER NOT NULL DEFAULT 0)`,
			`CREATE INDEX IF NOT EXISTS payloads_session ON payloads(session,status)`,
			`CREATE TABLE IF NOT EXISTS donors (slot INTEGER PRIMARY KEY, id TEXT NOT NULL, paths TEXT NOT NULL, known INTEGER NOT NULL)`,
		} {
			if _, err := c.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
		var key []byte
		if err := c.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='key'`).Scan(&key); err == sql.ErrNoRows {
			key = make([]byte, 32)
			if _, err := rand.Read(key); err != nil {
				return err
			}
			if _, err := c.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('key',?),('created',?),('schema','dp-adoption/1')`, key, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		if len(key) != 32 {
			return errors.New("invalid collector key")
		}
		s.key = key
		return nil
	})
	if err != nil {
		db.Close()
		return nil, errors.New("collector initialization failed")
	}
	return s, nil
}

// Close releases the hook process's connection.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) transaction(ctx context.Context, fn func(*sql.Conn) error) error {
	c, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer c.Close()
	if _, err = c.ExecContext(ctx, `PRAGMA busy_timeout=25`); err != nil {
		return err
	}
	if _, err = c.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = c.ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	if err = fn(c); err != nil {
		return err
	}
	if _, err = c.ExecContext(ctx, `COMMIT`); err != nil {
		return err
	}
	committed = true
	return nil
}

func (s *Store) hash(kind, value string) string {
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte(kind))
	h.Write([]byte{0})
	h.Write([]byte(value))
	return hex.EncodeToString(h.Sum(nil))
}

func counter(ctx context.Context, c *sql.Conn, name string, increment bool) (int64, error) {
	if _, err := c.ExecContext(ctx, `INSERT OR IGNORE INTO meta(key,value) VALUES(?, '0')`, name); err != nil {
		return 0, err
	}
	if increment {
		if _, err := c.ExecContext(ctx, `UPDATE meta SET value=CAST(value AS INTEGER)+1 WHERE key=?`, name); err != nil {
			return 0, err
		}
	}
	var text string
	if err := c.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, name).Scan(&text); err != nil {
		return 0, err
	}
	return strconv.ParseInt(text, 10, 64)
}

// Start records exactly one ordinal for an invocation, including retries of the
// hook. A payload boundary uses the current ordinal, not completion order.
func (s *Store) Start(ctx context.Context, h Hook) error {
	session, id := s.hash("session", h.Session), s.hash("call", h.ID)
	return s.transaction(ctx, func(c *sql.Conn) error {
		var found int
		err := c.QueryRowContext(ctx, `SELECT 1 FROM calls WHERE session=? AND id=?`, session, id).Scan(&found)
		if err == nil {
			_, err = counter(ctx, c, "duplicate_pre", true)
			return err
		}
		if err != sql.ErrNoRows {
			return err
		}
		var n int
		if err = c.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
			return err
		}
		if n >= maxSessions {
			var exists int
			if c.QueryRowContext(ctx, `SELECT 1 FROM sessions WHERE id=?`, session).Scan(&exists) != nil {
				return errors.New("session capacity")
			}
		}
		if _, err = c.ExecContext(ctx, `INSERT OR IGNORE INTO sessions(id,seq) VALUES(?,0)`, session); err != nil {
			return err
		}
		if _, err = c.ExecContext(ctx, `UPDATE sessions SET seq=seq+1 WHERE id=?`, session); err != nil {
			return err
		}
		var seq int64
		if err = c.QueryRowContext(ctx, `SELECT seq FROM sessions WHERE id=?`, session).Scan(&seq); err != nil {
			return err
		}
		if _, err = c.ExecContext(ctx, `INSERT INTO calls(session,id,seq) VALUES(?,?,?)`, session, id, seq); err != nil {
			return err
		}
		// Late or interrupted observations remain unknown after this retention
		// boundary; they can never be mistaken for ten completed negative calls.
		if _, err = c.ExecContext(ctx, `UPDATE payloads SET status='unknown' WHERE session=? AND status='pending' AND boundary<?`, session, seq-256); err != nil {
			return err
		}
		if _, err = c.ExecContext(ctx, `DELETE FROM calls WHERE session=? AND seq<?`, session, seq-256); err != nil {
			return err
		}
		_, err = counter(ctx, c, "pre", true)
		return err
	})
}

// Publish chooses the comparison event before subsequent observations exist.
// The reservoir and window row land in the same transaction, so concurrent
// sessions cannot pick themselves or double-advance the sampling population.
func (s *Store) Publish(ctx context.Context, h Hook, event string, at time.Time, refs []Reference) error {
	paths, known := s.Resolver.References(refs, h.CWD)
	for i := range paths {
		paths[i] = s.hash("path", paths[i])
	}
	data, _ := json.Marshal(paths)
	session, id := s.hash("session", h.Session), s.hash("event", event)
	return s.transaction(ctx, func(c *sql.Conn) error {
		var exists int
		if err := c.QueryRowContext(ctx, `SELECT 1 FROM payloads WHERE id=?`, id).Scan(&exists); err == nil {
			return nil
		} else if err != sql.ErrNoRows {
			return err
		}
		var n int
		if err := c.QueryRowContext(ctx, `SELECT COUNT(*) FROM payloads`).Scan(&n); err != nil {
			return err
		}
		if n >= maxEvents {
			return errors.New("payload capacity")
		}
		status := "pending"
		var boundary int64
		if err := c.QueryRowContext(ctx, `SELECT seq FROM sessions WHERE id=?`, session).Scan(&boundary); err == sql.ErrNoRows {
			status = "unknown"
		} else if err != nil {
			return err
		}
		var origin int
		if c.QueryRowContext(ctx, `SELECT 1 FROM calls WHERE session=? AND id=?`, session, s.hash("call", h.ID)).Scan(&origin) != nil {
			status = "unknown"
		}
		if !known {
			status = "unknown"
		}
		seen, err := counter(ctx, c, "offers", false)
		if err != nil {
			return err
		}
		baseID, basePaths := "", "[]"
		var poolSize int
		if err = c.QueryRowContext(ctx, `SELECT COUNT(*) FROM donors`).Scan(&poolSize); err != nil {
			return err
		}
		if poolSize != 0 {
			choice, err := s.Draw(int64(poolSize))
			if err != nil {
				return err
			}
			var baseKnown int
			if err = c.QueryRowContext(ctx, `SELECT id,paths,known FROM donors ORDER BY slot LIMIT 1 OFFSET ?`, choice).Scan(&baseID, &basePaths, &baseKnown); err != nil {
				return err
			}
			if baseKnown == 0 {
				status = "unknown"
			}
		}
		if _, err = c.ExecContext(ctx, `INSERT INTO payloads(id,session,timestamp_ns,boundary,paths,base_paths,base_id,status) VALUES(?,?,?,?,?,?,?,?)`, id, session, at.UnixNano(), boundary, string(data), basePaths, baseID, status); err != nil {
			return err
		}
		if seen < donorLimit {
			_, err = c.ExecContext(ctx, `INSERT INTO donors(slot,id,paths,known) VALUES(?,?,?,?)`, seen, id, string(data), boolInt(known))
		} else {
			choice, e := s.Draw(seen + 1)
			if e != nil {
				return e
			}
			if choice < donorLimit {
				_, err = c.ExecContext(ctx, `UPDATE donors SET id=?,paths=?,known=? WHERE slot=?`, id, string(data), boolInt(known), choice)
			}
		}
		if err != nil {
			return err
		}
		_, err = counter(ctx, c, "offers", true)
		return err
	})
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// Finish observes a tool invocation once. Exactly the next ten started calls
// must complete before either missing path match can become a negative label.
func (s *Store) Finish(ctx context.Context, h Hook) error {
	paths, known := Files(h)
	cwd, cwdKnown := WorkingDirectory(h)
	known = known && cwdKnown
	keys := map[string]bool{}
	for _, path := range paths {
		key, ok := s.Resolver.File(path, cwd)
		if !ok {
			known = false
			continue
		}
		keys[s.hash("path", key)] = true
	}
	session, id := s.hash("session", h.Session), s.hash("call", h.ID)
	return s.transaction(ctx, func(c *sql.Conn) error {
		var seq int64
		var finished int
		err := c.QueryRowContext(ctx, `SELECT seq,finished FROM calls WHERE session=? AND id=?`, session, id).Scan(&seq, &finished)
		if err == sql.ErrNoRows {
			if _, err = c.ExecContext(ctx, `UPDATE payloads SET status='unknown' WHERE session=? AND status='pending'`, session); err != nil {
				return err
			}
			_, err = counter(ctx, c, "missing_pre", true)
			return err
		}
		if err != nil {
			return err
		}
		if finished != 0 {
			_, err = counter(ctx, c, "duplicate_post", true)
			return err
		}
		if _, err = c.ExecContext(ctx, `UPDATE calls SET finished=1 WHERE session=? AND id=?`, session, id); err != nil {
			return err
		}
		rows, err := c.QueryContext(ctx, `SELECT id,paths,base_paths,base_id,touch_seen,base_touch_seen,uncertain,completed FROM payloads WHERE session=? AND status='pending' AND boundary<? AND boundary+?>=?`, session, seq, windowSize, seq)
		if err != nil {
			return err
		}
		type update struct {
			baseID                                     string
			id, paths, base                            string
			adopted, baseAdopted, uncertain, completed int
		}
		var updates []update
		for rows.Next() {
			var u update
			if err = rows.Scan(&u.id, &u.paths, &u.base, &u.baseID, &u.adopted, &u.baseAdopted, &u.uncertain, &u.completed); err != nil {
				rows.Close()
				return err
			}
			updates = append(updates, u)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, u := range updates {
			if intersects(keys, u.paths) {
				u.adopted = 1
			}
			if intersects(keys, u.base) {
				u.baseAdopted = 1
			}
			if !known {
				u.uncertain = 1
			}
			u.completed++
			status := "pending"
			var adopted, baseline any
			if u.completed == windowSize {
				if u.adopted != 0 || u.uncertain == 0 {
					adopted = u.adopted
				}
				if u.baseID != "" && (u.baseAdopted != 0 || u.uncertain == 0) {
					baseline = u.baseAdopted
				}
				switch {
				case adopted == nil:
					status = "unknown"
				case u.baseID == "":
					status = "unpaired"
				case baseline == nil:
					status = "unknown"
				default:
					status = "complete"
				}
			}
			if _, err = c.ExecContext(ctx, `UPDATE payloads SET touch_seen=?,base_touch_seen=?,adopted=?,base_adopted=?,uncertain=?,completed=?,status=? WHERE id=?`, u.adopted, u.baseAdopted, adopted, baseline, u.uncertain, u.completed, status, u.id); err != nil {
				return err
			}
		}
		_, err = counter(ctx, c, "post", true)
		return err
	})
}

func intersects(keys map[string]bool, encoded string) bool {
	var paths []string
	if json.Unmarshal([]byte(encoded), &paths) != nil {
		return false
	}
	for _, path := range paths {
		if keys[path] {
			return true
		}
	}
	return false
}

// Invalid records a capture gap. Without a trustworthy session it invalidates
// every pending window rather than assigning a global gap to an arbitrary one.
func (s *Store) Invalid(ctx context.Context, session string) error {
	return s.transaction(ctx, func(c *sql.Conn) error {
		query := `UPDATE payloads SET status='unknown' WHERE status='pending'`
		var args []any
		if session != "" {
			query += ` AND session=?`
			args = append(args, s.hash("session", session))
		}
		if _, err := c.ExecContext(ctx, query, args...); err != nil {
			return err
		}
		_, err := counter(ctx, c, "invalid", true)
		return err
	})
}

// Summary reports raw evidence, not a verdict or an automatic retirement.
type Summary struct {
	GapSeen         bool             `json:"coverage_gap_seen"`
	Created         string           `json:"created"`
	Counters        map[string]int64 `json:"counters"`
	Statuses        map[string]int   `json:"statuses"`
	Complete        int              `json:"complete"`
	Adopted         int              `json:"adopted"`
	BaselineAdopted int              `json:"baseline_adopted"`
	AdoptionRate    *float64         `json:"adoption_rate"`
	BaselineRate    *float64         `json:"baseline_rate"`
}

// Summary reads a bounded time slice; the caller supplies the observed install
// time, not an inferred deployment timestamp or the first source commit.
func (s *Store) Summary(ctx context.Context, start, end time.Time) (Summary, error) {
	out := Summary{GapSeen: GapSeen(s.dir), Counters: map[string]int64{}, Statuses: map[string]int{}}
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='created'`).Scan(&out.Created); err != nil {
		return out, err
	}
	for _, name := range []string{"pre", "post", "offers", "invalid", "missing_pre", "duplicate_pre", "duplicate_post"} {
		var value string
		err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, name).Scan(&value)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return out, err
		}
		out.Counters[name], _ = strconv.ParseInt(value, 10, 64)
	}
	startNS := start.UnixNano()
	if start.IsZero() {
		startNS = math.MinInt64
	}
	rows, err := s.db.QueryContext(ctx, `SELECT status,COUNT(*),COALESCE(SUM(adopted),0),COALESCE(SUM(base_adopted),0) FROM payloads WHERE timestamp_ns>=? AND timestamp_ns<? GROUP BY status`, startNS, end.UnixNano())
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n, a, b int
		if err = rows.Scan(&status, &n, &a, &b); err != nil {
			return out, err
		}
		out.Statuses[status] = n
		if status == "complete" {
			out.Complete = n
			out.Adopted = a
			out.BaselineAdopted = b
		}
	}
	if out.Complete > 0 && !out.GapSeen {
		a, b := float64(out.Adopted)/float64(out.Complete), float64(out.BaselineAdopted)/float64(out.Complete)
		out.AdoptionRate = &a
		out.BaselineRate = &b
	}
	return out, rows.Err()
}
