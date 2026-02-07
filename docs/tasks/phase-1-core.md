# Phase 1: Core (Collect + Store)

Foundation layer. Everything else depends on this.

## Tasks

### 1.1 Project initialization
- [x] `go mod init` with module path
- [x] Add dependencies to `go.mod` (cobra, sqlite, uuid)
- [x] Create `Makefile` with build, test, vet targets
- [x] Create `.gitignore`

### 1.2 Define core types (`internal/model/model.go`)
- [x] `Desire` struct with all fields from the data model
- [x] `Path` struct for aggregated patterns
- [x] `Alias` struct for tool name mappings
- [x] JSON tags on all exported fields
- [x] `time.Time` for timestamps, `json.RawMessage` for tool_input and metadata

### 1.3 Storage interface (`internal/store/store.go`)
- [x] `Store` interface with methods:
  - `RecordDesire(ctx, Desire) error`
  - `ListDesires(ctx, ListOpts) ([]Desire, error)`
  - `GetPaths(ctx, PathOpts) ([]Path, error)`
  - `SetAlias(ctx, from, to string) error`
  - `GetAliases(ctx) ([]Alias, error)`
  - `Stats(ctx) (Stats, error)`
  - `Close() error`
- [x] `ListOpts` struct (Since, Source, ToolName, Limit)
- [x] `PathOpts` struct (Top, Since)
- [x] `Stats` struct (TotalDesires, UniquePaths, TopSources)

### 1.4 SQLite implementation (`internal/store/sqlite.go`)
- [x] `New(dbPath string) (*SQLiteStore, error)` constructor
- [x] Auto-create `~/.dp/` directory if missing
- [x] Schema migration (version table + `desires` and `aliases` tables)
- [x] Implement all `Store` interface methods
- [x] Indexes on `tool_name`, `source`, `timestamp`

### 1.5 Record logic (`internal/record/record.go`)
- [x] `Record(ctx, store Store, input io.Reader, source string) error`
- [x] Parse JSON from reader into flexible map
- [x] Extract known fields, put remainder into `metadata`
- [x] Only `tool_name` is required; return error if missing
- [x] Generate UUID and timestamp if not provided

### 1.6 CLI scaffolding (`internal/cli/`)
- [x] `root.go` - Root command with `--db` and `--json` global flags
- [x] `record.go` - `dp record [--source NAME]` reads stdin, calls record logic
- [x] `cmd/dp/main.go` - Entry point calling `cli.Execute()`

### 1.7 Tests
- [x] `internal/model/` - JSON marshal/unmarshal round-trip
- [x] `internal/store/` - SQLite CRUD operations using `t.TempDir()`
- [x] `internal/record/` - Parsing various JSON inputs (minimal, full, malformed)

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
