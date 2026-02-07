# Phase 1: Core (Collect + Store)

Foundation layer. Everything else depends on this.

## Tasks

### 1.1 Project initialization
- [ ] `go mod init` with module path
- [ ] Add dependencies to `go.mod` (cobra, sqlite, uuid)
- [ ] Create `Makefile` with build, test, vet targets
- [ ] Create `.gitignore`

### 1.2 Define core types (`internal/model/model.go`)
- [ ] `Desire` struct with all fields from the data model
- [ ] `Path` struct for aggregated patterns
- [ ] `Alias` struct for tool name mappings
- [ ] JSON tags on all exported fields
- [ ] `time.Time` for timestamps, `json.RawMessage` for tool_input and metadata

### 1.3 Storage interface (`internal/store/store.go`)
- [ ] `Store` interface with methods:
  - `RecordDesire(ctx, Desire) error`
  - `ListDesires(ctx, ListOpts) ([]Desire, error)`
  - `GetPaths(ctx, PathOpts) ([]Path, error)`
  - `SetAlias(ctx, from, to string) error`
  - `GetAliases(ctx) ([]Alias, error)`
  - `Stats(ctx) (Stats, error)`
  - `Close() error`
- [ ] `ListOpts` struct (Since, Source, ToolName, Limit)
- [ ] `PathOpts` struct (Top, Since)
- [ ] `Stats` struct (TotalDesires, UniquePaths, TopSources)

### 1.4 SQLite implementation (`internal/store/sqlite.go`)
- [ ] `New(dbPath string) (*SQLiteStore, error)` constructor
- [ ] Auto-create `~/.dp/` directory if missing
- [ ] Schema migration (version table + `desires` and `aliases` tables)
- [ ] Implement all `Store` interface methods
- [ ] Indexes on `tool_name`, `source`, `timestamp`

### 1.5 Record logic (`internal/record/record.go`)
- [ ] `Record(ctx, store Store, input io.Reader, source string) error`
- [ ] Parse JSON from reader into flexible map
- [ ] Extract known fields, put remainder into `metadata`
- [ ] Only `tool_name` is required; return error if missing
- [ ] Generate UUID and timestamp if not provided

### 1.6 CLI scaffolding (`internal/cli/`)
- [ ] `root.go` - Root command with `--db` and `--json` global flags
- [ ] `record.go` - `dp record [--source NAME]` reads stdin, calls record logic
- [ ] `cmd/dp/main.go` - Entry point calling `cli.Execute()`

### 1.7 Tests
- [ ] `internal/model/` - JSON marshal/unmarshal round-trip
- [ ] `internal/store/` - SQLite CRUD operations using `t.TempDir()`
- [ ] `internal/record/` - Parsing various JSON inputs (minimal, full, malformed)

## Done when

```bash
echo '{"tool_name":"read_file","error":"unknown tool"}' | ./dp record --source test
# exits 0, desire stored in ~/.dp/desires.db
```

## Depends on

Nothing - this is the foundation.

## Blocks

- Phase 2 (Reporting)
- Phase 3 (Suggestions & Aliases)
- Phase 4 (Polish)
