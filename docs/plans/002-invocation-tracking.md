# 002 - Tool Invocation Tracking for dp

## Context

dp currently tracks only **failed** tool calls ("desires") via `PostToolUseFailure` hooks. This plan adds tracking of **all** tool invocations to detect behavioral patterns like "first tool call in a multi-call turn didn't achieve its intended result." This requires a new invocation model, a source plugin abstraction for multi-tool support, and export functionality designed for analytics.

**Multi-tool support is a first-class concern.** The plugin system must work cleanly for Claude Code, Gemini CLI, Kiro CLI, OpenCode, and any future AI coding tool. Source-specific concepts (transcript paths, hook event names, tool use IDs) must never leak into core data models or schema — they belong in metadata.

Data comes from tool hooks (JSON on stdin). No tool output storage for now. Implicit timestamp-based sequencing; turn reconstruction is backlog.

## Design Principles

### 1. Universal core, source-specific metadata

The Invocation model and schema contain only fields that every AI coding tool shares: tool name, instance/session ID, error state, working directory, timestamp. Everything source-specific goes into a `metadata` JSON column. This prevents schema bloat as new sources are added and avoids NULL-heavy columns.

### 2. Plugins own their parsing

Source plugins receive raw bytes from stdin and handle their own JSON structure. The ingest layer doesn't pre-parse or assume JSON shape. This allows future sources with different input formats or nested structures.

### 3. Plugin-driven installation

Each source plugin optionally knows how to install its own hooks. `dp init` delegates to the plugin rather than hardcoding tool-specific config file formats. This scales to N tools without modifying init logic.

### 4. Fields are a superset for convergence

The `Fields` struct returned by plugins contains everything needed to create both an `Invocation` and a `Desire`. This enables the future convergence where `dp ingest` replaces `dp record` as the single entry point.

## Design Considerations (inform architecture, not implemented this phase)

### Tool Allowlist

Users should be able to specify which tools to track (e.g., only `Read`, `Grep`, `Bash`) rather than recording every invocation. This affects:
- **Config**: add `track_tools []string` to `~/.dp/config.toml` (empty = track all)
- **Filtering point**: the CLI layer (`dp ingest` RunE) checks config before calling `ingest.Ingest()`. Keep the ingest package unaware of filtering — that's a policy concern, not a parsing concern.
- **Applies to invocations only** — desires (failures) should always be recorded regardless of allowlist, since failures are inherently noteworthy.

**What this means for the current implementation**: Config struct must be extensible (it already is). The `dp ingest` command already has access to config via the root command's `PersistentPreRun`. No architectural changes needed — just a future flag check before the store call.

### Plugin Abstraction Covers Existing Failure Tracking

The source plugin system should be the **single abstraction** for how external AI tools talk to dp — not just for new invocation tracking, but also for the existing desires (failure) path. This means:
- `Source.Extract()` returns `Fields` that contain everything needed for BOTH `Invocation` and `Desire` creation
- **Future**: `dp record --source claude-code` could delegate to the ClaudeCode plugin for field extraction instead of using its own bespoke `knownFields` map in `record.go`
- **Eventual convergence**: `dp ingest` could handle both tables — when `is_error=true`, also write a Desire for backward compat. `dp record` becomes a legacy alias.

**What this means for the current implementation**: Design `Fields` to be a superset of what both `Desire` and `Invocation` need. Include `ToolInput` in `Fields` even though invocations don't store it yet — desires need it, and future invocations may want it.

## Implementation

### 1. Source Plugin Interface — `internal/source/`

**New files:** `source.go`, `claudecode.go`, `source_test.go`

**Source interface:**
```go
// Source extracts normalized fields from a specific AI tool's hook output.
type Source interface {
    // Name returns the source identifier (e.g., "claude-code", "gemini-cli").
    Name() string

    // Extract parses raw hook output bytes and returns normalized fields.
    // The plugin owns all parsing — the caller passes raw stdin bytes.
    Extract(raw []byte) (Fields, error)
}
```

**Fields struct — universal fields only:**
```go
// Fields holds normalized data extracted by a source plugin.
// Only truly universal fields are top-level. Source-specific data goes in Extra.
type Fields struct {
    ToolName   string                       // Required: what tool was called
    InstanceID string                       // Session/instance identifier (source-specific name normalized)
    ToolInput  json.RawMessage              // Tool parameters (optional, for desire convergence)
    CWD        string                       // Working directory (optional)
    Error      string                       // Error message if failed (empty = success)
    Extra      map[string]json.RawMessage   // Everything source-specific: hook_event, transcript_path, tool_use_id, permission_mode, etc.
}
```

**Why Extra instead of top-level fields:** Claude Code sends `transcript_path`, `hook_event_name`, `tool_use_id`. Gemini CLI won't have these. Kiro will have different fields. Putting source-specific data in `Extra` means the core model never needs to change when a new source is added. These fields are still accessible — they're marshaled into `metadata` JSON in the invocations table and can be queried.

**Registry pattern:** `Register(s Source)` / `Get(name string) (Source, bool)` / `Names() []string` — plugins self-register via `init()`.

**dp generates** id (UUID), timestamp, host_id (os.Hostname). Plugins provide everything else.

**Optional Installer interface:**
```go
// Installer is optionally implemented by sources that can auto-configure hooks
// in their AI tool's settings file.
type Installer interface {
    // InstallHooks configures the AI tool to send hook data to dp.
    // configDir is the tool's config directory (e.g., ~/.claude for Claude Code).
    // If empty, the plugin uses the tool's default location.
    InstallHooks(configDir string) error
}
```

**ClaudeCode plugin:** maps `session_id` → `InstanceID`, extracts `tool_name`, `tool_input`, `cwd`, `error` into Fields. Puts `tool_use_id`, `transcript_path`, `hook_event_name`, `permission_mode`, and any unknown keys into `Extra`. Implements `Installer` for `dp init --source claude-code`.

### 2. Invocation Model — `internal/model/model.go`

Add `Invocation` type alongside existing Desire/Path/Alias — **universal fields only:**

```go
// Invocation represents a single tool call (successful or failed) from an AI
// coding assistant. Source-specific fields are stored in Metadata.
type Invocation struct {
    ID         string          `json:"id"`
    Source     string          `json:"source"`
    InstanceID string          `json:"instance_id"`
    HostID     string          `json:"host_id"`
    ToolName   string          `json:"tool_name"`
    IsError    bool            `json:"is_error"`
    Error      string          `json:"error,omitempty"`
    CWD        string          `json:"cwd,omitempty"`
    Timestamp  time.Time       `json:"timestamp"`
    Metadata   json.RawMessage `json:"metadata,omitempty"`
}
```

Source-specific fields (`tool_use_id`, `hook_event`, `transcript_path`) live in `Metadata` JSON, not as top-level struct fields. This keeps the model clean for any source.

Flat, snake_case, omitempty on optionals — directly DuckDB/Parquet-ready.

### 3. Store Changes — `internal/store/`

**store.go** — Add to interface:
- `RecordInvocation(ctx, inv) error`
- `ListInvocations(ctx, InvocationOpts) ([]Invocation, error)`
- `InvocationStats(ctx) (InvocationStats, error)`
- New types: `InvocationOpts` (Since, Source, InstanceID, ToolName, ErrorsOnly, Limit), `InvocationStats`

**sqlite.go** — Schema migration v2 — **universal columns only:**
```sql
CREATE TABLE invocations (
    id          TEXT PRIMARY KEY,
    source      TEXT NOT NULL,
    instance_id TEXT NOT NULL,
    host_id     TEXT NOT NULL,
    tool_name   TEXT NOT NULL,
    is_error    INTEGER NOT NULL DEFAULT 0,
    error       TEXT,
    cwd         TEXT,
    timestamp   TEXT NOT NULL,
    metadata    TEXT
);
CREATE INDEX idx_inv_source ON invocations(source);
CREATE INDEX idx_inv_instance ON invocations(instance_id);
CREATE INDEX idx_inv_tool ON invocations(tool_name);
CREATE INDEX idx_inv_timestamp ON invocations(timestamp);
CREATE INDEX idx_inv_error ON invocations(is_error);
```

No `tool_use_id`, `hook_event`, or `transcript_path` columns. Those go in `metadata` JSON. This mirrors the existing `desires` table pattern where unknown fields are collected into `metadata`.

Implementations follow existing patterns (ExecContext for insert, dynamic WHERE for list, aggregate queries for stats). Add `boolToInt()` helper.

**Update fakeStore** in `record_test.go` with stub methods so existing tests compile.

### 4. Ingest Package — `internal/ingest/`

**New files:** `ingest.go`, `ingest_test.go`

Parallel to `internal/record/` but routes through source plugins:

```
stdin bytes → source.Get(name).Extract(raw) → build Invocation → store.RecordInvocation()
```

- `Ingest(ctx, store, reader, sourceName) (Invocation, error)`
- Reads raw bytes from reader, passes to plugin's `Extract()`
- Generates id (UUID), timestamp (time.Now), host_id (os.Hostname)
- Derives `is_error` from `Fields.Error != ""`
- Marshals `Fields.Extra` into `Invocation.Metadata`

### 5. CLI Changes

**New file: `internal/cli/ingest.go`** — `dp ingest` command
- `--source` flag (required) — selects plugin by name
- Reads stdin, calls `ingest.Ingest()`
- Supports `--json` global flag
- Plugin registration via blank import in root or ingest command

**Modified: `internal/cli/export.go`**
- Add `--type desires|invocations` flag (default: `desires` for backward compat)
- New `writeInvocationsJSON()` and `writeInvocationsCSV()` functions
- CSV columns: id, source, instance_id, host_id, tool_name, is_error, error, cwd, timestamp, metadata

**Modified: `internal/cli/init_cmd.go`**
- Replace `--claude-code` flag with `--source NAME` flag
- Delegate to plugin's `Installer` interface if implemented
- Refactor `mergeHook()` → `mergeHookEvent(settings, eventName, hook)` (parameterize event name)
- Refactor `hasDPHook()` → `hasDPHookCommand(entries, command)` (parameterize command)
- Claude Code installer sets up THREE hooks:
  1. `PostToolUseFailure` → `dp record --source claude-code` (existing, backward compat)
  2. `PostToolUse` → `dp ingest --source claude-code` (new, async)
  3. `PostToolUseFailure` → `dp ingest --source claude-code` (new, async — failures as invocations too)
- Keep `--claude-code` as a deprecated alias for `--source claude-code`

**Modified: `internal/cli/stats.go`**
- Add `--invocations` flag to show invocation stats instead of desire stats

### 6. Documentation Updates

- **AGENTS.md** — Add `internal/source/` and `internal/ingest/` package descriptions, document plugin conventions
- **Code comments** — All exported symbols get doc comments per conventions

### 7. Backlog (documented, not implemented)

**Near-term (informed by design considerations above):**
1. Tool allowlist — `track_tools` config key, filter in `dp ingest` CLI layer before store call
2. Migrate `dp record` to use source plugins — replace bespoke `knownFields` parsing with `Source.Extract()`, then map `Fields` → `Desire`
3. Converge desires into ingest path — `dp ingest` writes to both tables when `is_error=true`, `dp record` becomes a legacy alias
4. `dp sources` command — list registered plugins, their names, and whether they implement Installer

**Medium-term:**
5. Additional source plugins: Gemini CLI, Kiro CLI, OpenCode, Cursor
6. Turn reconstruction via transcript parsing
7. Tool output capture (`tool_response` gated by config flag)
8. `tool_input` storage for invocations
9. Invocation-specific analysis commands (session timeline, pattern detection)
10. Enrichment pipeline (`dp enrich`)

**Longer-term:**
11. Central service upload / sidecar plugin (`dp sync`)
12. Cross-session pattern detection

## Implementation Order

1. `internal/source/` — plugin interface + registry + Claude Code plugin + tests
2. `internal/model/model.go` — add Invocation type
3. `internal/store/` — interface + migration v2 + implementations + tests (update fakeStore)
4. `internal/ingest/` — ingest package + tests
5. `internal/cli/ingest.go` — dp ingest command
6. `internal/cli/export.go` — add --type flag
7. `internal/cli/init_cmd.go` — refactor to plugin-driven init + add PostToolUse hook setup
8. `internal/cli/stats.go` — add --invocations flag
9. AGENTS.md update

## Verification

```bash
# Build
make build

# Unit tests
make test

# Smoke test: ingest a mock PostToolUse event
echo '{"tool_name":"Read","session_id":"sess-1","tool_use_id":"toolu_abc","cwd":"/tmp","hook_event_name":"PostToolUse"}' | ./dp ingest --source claude-code

# Verify it's stored
./dp export --type invocations --format json

# Verify source-specific fields are in metadata (not top-level columns)
./dp export --type invocations --format json | grep -q '"metadata".*tool_use_id'

# Verify init delegates to plugin
./dp init --source claude-code  # (against a temp settings file in tests)

# Verify stats
./dp stats --invocations

# Verify backward compat: existing desires still work
echo '{"tool_name":"bad_tool","error":"not found"}' | ./dp record --source test
./dp list
./dp export --format json
```

## Architectural Decisions

### Why universal columns + metadata JSON (not source-specific columns)

Adding `tool_use_id`, `hook_event`, `transcript_path` as schema columns means:
- Every non-Claude-Code source leaves those columns NULL
- Every new source with unique fields needs a schema migration
- The invocations table becomes a union of all sources' vocabularies

Using `metadata` JSON instead means:
- Schema is stable across source additions
- Source-specific data is still queryable via SQLite JSON functions
- Mirrors the proven pattern already used in the `desires` table
- DuckDB/Parquet export naturally handles the JSON column

### Why Extract([]byte) not Extract(map[string]json.RawMessage)

Pre-parsing JSON before handing to the plugin assumes:
- All sources use flat JSON (no nesting)
- All sources use JSON at all

Passing raw bytes lets each plugin own its parsing entirely. The cost is trivial (each plugin calls `json.Unmarshal` itself) and the flexibility gain is significant for future sources with different input shapes.

### Why optional Installer interface

Not all AI tools have configurable hook files. Some might use environment variables, CLI flags, or have no auto-configuration at all. Making installation optional via a separate interface means:
- Sources can exist without installation support
- `dp init --source foo` gives a clear error if the source doesn't support auto-install
- Each source encapsulates its own tool's config format
