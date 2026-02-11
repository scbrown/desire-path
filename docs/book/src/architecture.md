# Architecture

dp is a single-binary CLI built in Go with minimal dependencies. It uses a plugin architecture for source integrations, a SQLite database for storage, and Levenshtein-based similarity matching for suggestions. This page explains how the pieces fit together.

## Data Flow

```mermaid
graph LR
    A[AI Tool Hook] --> B[dp record/ingest]
    B --> C[Source.Extract]
    C --> D[Fields]
    D --> E[ingest.Ingest]
    E --> F[Invocation/Desire]
    F --> G[SQLite]
    G --> H[Queries]
    H --> I[CLI Output]
```

1. **Hook Trigger**: AI tool (e.g., Claude Code) fires a hook on tool call failure (or success, if full tracking is enabled)
2. **Command Execution**: Hook runs `dp record` or `dp ingest` with `--source <name>`, passing JSON via stdin
3. **Source Plugin**: The named source plugin's `Extract` method parses the JSON into universal `Fields`
4. **Normalization**: `ingest.Ingest` converts `Fields` to `Invocation` or `Desire`, generating UUID and timestamp
5. **Storage**: Data is written to SQLite database with WAL mode enabled
6. **Query**: CLI commands (`list`, `paths`, `inspect`, etc.) query the database
7. **Output**: Results rendered as table or JSON

## Core Components

### 1. Source Plugin System

Located in `internal/source/`.

**Registry Pattern**: Plugins self-register via `init()` functions. The `source` package maintains a global registry mapping source names to `Source` implementations.

```go
type Source interface {
    Name() string
    Extract(raw []byte) (*Fields, error)
}
```

**Fields Struct**: Universal representation of a tool call:

```go
type Fields struct {
    ToolName   string          // Required
    InstanceID string          // Optional: session/request ID
    ToolInput  json.RawMessage // Optional: raw input params
    CWD        string          // Optional: working directory
    Error      string          // Optional: error message
    Extra      map[string]json.RawMessage // Source-specific fields
}
```

**Installer Interface** (optional):

```go
type Installer interface {
    Install(opts InstallOpts) error
}
```

Allows `dp init --source <name>` to automatically configure hooks.

**Current Plugins**:
- `claude-code`: Parses Claude Code hook JSON, maps `session_id` → `InstanceID`, extracts `tool_use_id`/`transcript_path` into `Extra`

**Adding Plugins**: Create a new file `internal/source/<name>.go`, implement `Source`, call `Register()` in `init()`, import package in `cmd/dp/main.go`.

See [Writing a Plugin](./integrations/writing-plugins.md) for details.

### 2. Ingest Pipeline

Located in `internal/ingest/`.

**Function**: `Ingest(ctx, store, raw, sourceName)` orchestrates the pipeline:

1. Fetch source plugin by name
2. Call `Extract(raw)` to get `Fields`
3. Validate `ToolName` is non-empty
4. Convert `Fields` → `model.Invocation`
5. Generate UUID and timestamp if missing
6. Marshal `Extra` into `Metadata` JSON column
7. Write to database via `store.RecordInvocation()`

**Error Handling**: Returns descriptive errors if source is unknown, extraction fails, or storage fails.

### 3. Data Model

Located in `internal/model/`.

**Desire**: A failed tool call.

```go
type Desire struct {
    ID        string          // UUID
    ToolName  string          // Name of the tool that failed
    ToolInput json.RawMessage // Raw input params
    Error     string          // Error message
    Source    string          // Source plugin name (e.g., "claude-code")
    SessionID string          // Session/instance ID
    CWD       string          // Working directory
    Timestamp time.Time       // When it happened
    Metadata  json.RawMessage // Extra fields from plugin
}
```

**Path**: Aggregated pattern of repeated desires.

```go
type Path struct {
    ID        string    // Tool name (used as ID)
    Pattern   string    // Tool name
    Count     int       // Occurrences
    FirstSeen time.Time // First failure
    LastSeen  time.Time // Most recent failure
    AliasTo   string    // Alias target (if one exists)
}
```

**Alias**: Mapping from hallucinated tool name to real tool name.

```go
type Alias struct {
    From      string    // Hallucinated name
    To        string    // Real tool name
    CreatedAt time.Time // When alias was created
}
```

**Invocation**: Any tool call (success or failure), used for full tracking.

```go
type Invocation struct {
    ID         string          // UUID
    Source     string          // Source plugin name
    InstanceID string          // Session/request ID
    HostID     string          // Machine ID (future)
    ToolName   string          // Tool that was invoked
    IsError    bool            // Whether call failed
    Error      string          // Error message (if IsError)
    CWD        string          // Working directory
    Timestamp  time.Time       // When it happened
    Metadata   json.RawMessage // Extra fields
}
```

### 4. Storage Layer

Located in `internal/store/`.

**Interface**: `Store` defines the persistence API. Commands interact with `Store`, not directly with SQL.

```go
type Store interface {
    RecordDesire(ctx, Desire) error
    ListDesires(ctx, ListOpts) ([]Desire, error)
    GetPaths(ctx, PathOpts) ([]Path, error)
    SetAlias(ctx, from, to string) error
    GetAliases(ctx) ([]Alias, error)
    DeleteAlias(ctx, from string) (bool, error)
    Stats(ctx) (Stats, error)
    InspectPath(ctx, InspectOpts) (*InspectResult, error)
    RecordInvocation(ctx, Invocation) error
    ListInvocations(ctx, InvocationOpts) ([]Invocation, error)
    InvocationStats(ctx) (InvocationStatsResult, error)
    Close() error
}
```

**Implementation**: `sqliteStore` in `internal/store/sqlite.go`.

**Database**: Pure-Go SQLite via `modernc.org/sqlite` (no CGo, cross-compiles easily).

**Concurrency**: WAL mode enabled for concurrent reads and writes. Writers don't block readers.

**Schema**:

```sql
CREATE TABLE desires (
    id TEXT PRIMARY KEY,
    tool_name TEXT NOT NULL,
    tool_input TEXT,
    error TEXT NOT NULL,
    source TEXT,
    session_id TEXT,
    cwd TEXT,
    timestamp TEXT NOT NULL,
    metadata TEXT
);

CREATE TABLE invocations (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    instance_id TEXT,
    host_id TEXT,
    tool_name TEXT NOT NULL,
    is_error INTEGER NOT NULL,
    error TEXT,
    cwd TEXT,
    timestamp TEXT NOT NULL,
    metadata TEXT
);

CREATE TABLE aliases (
    from_name TEXT PRIMARY KEY,
    to_name TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE schema_version (
    version INTEGER NOT NULL
);
```

**Indexes**:
- `desires.tool_name` (for `GetPaths`, `InspectPath`)
- `desires.timestamp` (for time-based filtering)
- `invocations.tool_name`, `invocations.instance_id`, `invocations.timestamp`

**Migrations**: Managed in `sqlite.go` via `schema_version` table. New migrations bump the version and run idempotently.

### 5. Analysis Engine

Located in `internal/analyze/`.

**Similarity Matching**: Used by `dp similar` to find known tools similar to a hallucinated name.

**Algorithm**:

1. **Normalize**: Convert both strings to lowercase, split camelCase/underscores/hyphens into words
   - `readFile` → `"read file"`
   - `Read_File` → `"read file"`
   - `read-file` → `"read file"`

2. **Levenshtein Distance**: Compute edit distance between normalized strings

3. **Normalize Score**: `score = 1 - (distance / max_length)`

4. **Bonuses**:
   - **Prefix**: Add `0.1 * (common_prefix_length / max_length)`
   - **Suffix**: Add `0.05 * (common_suffix_length / max_length)`

5. **Filter**: Keep suggestions with `score >= threshold` (default: 0.5)

6. **Rank**: Sort by score descending, return top N (default: 5)

**Example**:

```
Hallucinated: "read_file"
Known tools: ["Read", "Write", "ReadFile", "EditFile"]

Normalized: "read file"

Scores:
- Read:     normalize("read") = "read"
            distance("read file", "read") = 5
            base = 1 - (5/9) = 0.44
            prefix = 0.1 * (4/9) = 0.044
            total = 0.484 (below threshold, filtered out)

- ReadFile: normalize("ReadFile") = "read file"
            distance("read file", "read file") = 0
            base = 1.0
            total = 1.0 ✓

- EditFile: normalize("EditFile") = "edit file"
            distance("read file", "edit file") = 4
            base = 1 - (4/9) = 0.56
            prefix = 0.1 * (0/9) = 0
            suffix = 0.05 * (5/9) = 0.028
            total = 0.588 ✓

Suggestions: [("ReadFile", 1.0), ("EditFile", 0.588)]
```

**Customization**: Set `known_tools` in config to override the default list:

```bash
dp config known_tools "Read,Write,Edit,Bash,CustomTool"
```

### 6. CLI Commands

Located in `cmd/dp/` and `internal/cli/`.

**Framework**: Cobra (`github.com/spf13/cobra`) for command parsing, flags, and help.

**Structure**:

```
cmd/dp/main.go         → entry point, registers commands
internal/cli/*.go      → command implementations
```

**Common Pattern**:

```go
func runList(cmd *cobra.Command, args []string) error {
    db := openDatabase()
    defer db.Close()

    opts := store.ListOpts{
        Since:    parseTime(since),
        Source:   source,
        Limit:    limit,
    }

    desires, err := db.ListDesires(cmd.Context(), opts)
    if err != nil {
        return err
    }

    if jsonOutput {
        return printJSON(desires)
    }
    return printTable(desires)
}
```

**Global Flags**:
- `--db PATH`: Override database path
- `--json`: Force JSON output

**Output Formats**:
- **Table**: Human-readable, uses `golang.org/x/term` for width detection
- **JSON**: Machine-readable, one JSON array or object

**Error Handling**: Commands return errors, Cobra prints them to stderr and exits with code 1.

## Dependencies

From `go.mod`:

```go
require (
    github.com/google/uuid v1.6.0           // UUID generation
    github.com/spf13/cobra v1.10.2          // CLI framework
    golang.org/x/term v0.39.0               // Terminal width detection
    modernc.org/sqlite v1.44.3              // Pure-Go SQLite
)
```

**Why These?**

- **uuid**: Standard, fast, no deps
- **cobra**: Best CLI framework in Go, used by kubectl, gh, etc.
- **term**: Stdlib extension for terminal queries
- **sqlite**: Pure Go (no CGo), cross-compiles to any platform, fast enough for millions of rows

**No Other Deps**: No ORM, no logging framework, no config parser beyond `encoding/json`, no HTTP libraries. dp is self-contained.

## Build and Release

**Makefile**:

```makefile
install:
    go install ./cmd/dp
```

**Releases**: Automated via GoReleaser (`.goreleaser.yml`). Pushes binary artifacts to GitHub Releases for Linux, macOS, Windows.

**Binary Size**: ~8MB (includes SQLite, CLI framework, compression libraries).

## Performance Characteristics

- **Desire recording**: ~5ms (database write + fsync)
- **Path aggregation**: ~10ms for 10k desires
- **Similarity matching**: ~1ms for 100 known tools
- **Database size**: ~1KB per desire, ~1.5KB per invocation
- **Query latency**: <1ms for indexed lookups, <10ms for full scans up to 100k rows

**Scaling**:

- SQLite handles millions of rows fine with proper indexes
- WAL mode keeps reads fast during writes
- If you exceed ~10M records, consider archiving old data or partitioning by date
- For multi-machine aggregation, export to JSON and load into a central database

## Security Notes

- Database file permissions: `0600` (user-only read/write)
- Config file permissions: `0644` (world-readable, but no secrets stored)
- Hooks run with user's shell environment—ensure `dp` binary is trusted
- No network access, no external API calls, no telemetry

## Extension Points

Want to extend dp? Here are the main interfaces:

1. **Source Plugins**: Add support for new AI tools (see [Writing a Plugin](./integrations/writing-plugins.md))
2. **Store Implementations**: Swap SQLite for Postgres, MySQL, etc. (implement `store.Store`)
3. **Analysis Algorithms**: Replace Levenshtein with ML embeddings, fuzzy matching, etc. (modify `internal/analyze`)
4. **Output Formats**: Add CSV, Markdown, HTML (modify CLI commands)
5. **Webhooks**: Add `dp serve` command to expose a REST API (new package)

All interfaces are defined in `internal/` packages—keep the public API minimal (`cmd/dp` is the only entry point).

## Code Layout

```
desire-path/
├── cmd/dp/                 # CLI entry point
├── internal/
│   ├── analyze/            # Similarity matching
│   ├── cli/                # Command implementations
│   ├── config/             # Config file parsing
│   ├── ingest/             # Data ingestion pipeline
│   ├── model/              # Data types (Desire, Path, Alias, Invocation)
│   ├── source/             # Source plugin system
│   └── store/              # Storage interface + SQLite implementation
├── docs/book/              # This documentation (mdbook)
├── go.mod
├── Makefile
└── README.md
```

**Conventions**:

- `internal/` packages are not importable by external code (Go convention)
- Interfaces in `internal/store/` and `internal/source/` allow mocking for tests
- No init-time side effects except plugin registration
- Error messages include context: `"source plugin: operation: detail: error"`

## Testing

Run all tests:

```bash
go test ./...
```

**Test Coverage**:

- Unit tests for each package (`*_test.go` files)
- Integration tests using in-memory SQLite (`:memory:`)
- Example-based tests in `source` package for plugin validation

**No External Dependencies**: Tests don't need Docker, external databases, or network access. They run in <1s.

## Future Architecture Considerations

Potential improvements:

- **Multi-database aggregation**: Collect desires from multiple machines, merge into a central store
- **Streaming ingestion**: Replace hook-based recording with a long-running daemon that tails logs
- **Machine learning**: Train embeddings for better similarity matching
- **Distributed tracing**: Correlate tool calls across services using OpenTelemetry
- **Web UI**: Visualize paths, trends, session replays

These would require architectural changes (client/server split, service discovery, etc.), but the current design keeps things simple and fast for single-user CLI workflows.

## Next Steps

- Read the [source code](https://github.com/scbrown/desire-path)
- Write a [source plugin](./integrations/writing-plugins.md)
- Explore the [command reference](./commands/README.md)
- Check out the [Claude Code integration](./integrations/claude-code.md)
