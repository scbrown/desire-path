# Phase 5: Invocation Tracking

Adds tracking of all tool invocations (not just failures), with a plugin system for multi-tool support. See `docs/plans/002-invocation-tracking.md` for full architecture.

**Status: COMPLETE** — All tasks implemented and tested.

## Tasks

### 5.1 Source plugin interface (`internal/source/source.go`)
- [x] `Source` interface: `Name() string` + `Extract(raw []byte) (Fields, error)`
- [x] `Fields` struct with universal fields only: ToolName, InstanceID, ToolInput, CWD, Error, Extra map
- [x] `Installer` optional interface: `InstallHooks(configDir string) error`
- [x] Registry: `Register(s Source)` / `Get(name string) (Source, bool)` / `Names() []string`
- [x] Panic or error on duplicate registration

### 5.2 Claude Code plugin (`internal/source/claudecode.go`)
- [x] Implement `Source` interface
- [x] `Name()` returns `"claude-code"`
- [x] `Extract()` parses Claude Code hook JSON from raw bytes
- [x] Maps `session_id` → `InstanceID`, `tool_name` → `ToolName`, `cwd` → `CWD`, `error` → `Error`
- [x] Extracts `tool_input` into `ToolInput`
- [x] Puts `tool_use_id`, `transcript_path`, `hook_event_name`, `permission_mode` into `Extra`
- [x] Collects any unknown keys into `Extra`
- [x] Implements `Installer` — sets up PostToolUse + PostToolUseFailure hooks in `~/.claude/settings.json`
- [x] Self-registers via `init()`

### 5.3 Source plugin tests (`internal/source/source_test.go`)
- [x] Registry: register, get, names, duplicate panics
- [x] Claude Code Extract: full payload, minimal payload, missing tool_name error
- [x] Claude Code Extract: source-specific fields land in Extra (not top-level)
- [x] Claude Code Installer: creates hooks in temp settings file, idempotent re-run

### 5.4 Invocation model (`internal/model/model.go`)
- [x] Add `Invocation` struct: ID, Source, InstanceID, HostID, ToolName, IsError, Error, CWD, Timestamp, Metadata
- [x] JSON tags with omitempty on optional fields
- [x] No source-specific fields (hook_event, transcript_path, tool_use_id go in Metadata)

### 5.5 Store interface changes (`internal/store/store.go`)
- [x] Add `RecordInvocation(ctx, Invocation) error`
- [x] Add `ListInvocations(ctx, InvocationOpts) ([]Invocation, error)`
- [x] Add `InvocationStats(ctx) (InvocationStats, error)`
- [x] `InvocationOpts` struct: Since, Source, InstanceID, ToolName, ErrorsOnly, Limit
- [x] `InvocationStats` struct: totals, unique tools, top sources, top tools, time windows, date range

### 5.6 SQLite migration v2 (`internal/store/sqlite.go`)
- [x] Schema version 2 migration
- [x] `invocations` table with universal columns only: id, source, instance_id, host_id, tool_name, is_error, error, cwd, timestamp, metadata
- [x] Indexes on source, instance_id, tool_name, timestamp, is_error
- [x] `RecordInvocation` implementation (ExecContext INSERT)
- [x] `ListInvocations` implementation (dynamic WHERE clauses)
- [x] `InvocationStats` implementation (aggregate queries)
- [x] `boolToInt()` helper for is_error

### 5.7 Update fakeStore for test compilation
- [x] Add stub `RecordInvocation`, `ListInvocations`, `InvocationStats` to fakeStore in `record_test.go`
- [x] Existing tests still pass with no behavior changes

### 5.8 Store tests for invocations
- [x] RecordInvocation: insert and retrieve round-trip
- [x] ListInvocations: filter by source, instance_id, tool_name, errors_only, since, limit
- [x] InvocationStats: correct totals, top tools, time windows
- [x] Metadata JSON preserved correctly through insert/retrieve cycle
- [x] Migration v2 applied cleanly on fresh DB and on existing v1 DB

### 5.9 Ingest package (`internal/ingest/`)
- [x] `Ingest(ctx, store, reader, sourceName) (Invocation, error)`
- [x] Reads raw bytes from reader
- [x] Looks up source plugin by name, returns error if not found
- [x] Calls `plugin.Extract(raw)` to get Fields
- [x] Generates UUID, timestamp, host_id
- [x] Derives `is_error` from `Fields.Error != ""`
- [x] Marshals `Fields.Extra` → `Invocation.Metadata`
- [x] Calls `store.RecordInvocation()`

### 5.10 Ingest tests (`internal/ingest/ingest_test.go`)
- [x] Full Claude Code payload → correct Invocation fields
- [x] Source-specific fields in Extra end up in Metadata JSON
- [x] Missing tool_name → error
- [x] Unknown source name → error
- [x] UUID and timestamp auto-generated

### 5.11 CLI: `dp ingest` command (`internal/cli/ingest.go`)
- [x] `--source` flag (required)
- [x] Reads stdin, calls `ingest.Ingest()`
- [x] Supports `--json` global flag for output
- [x] Error messages for missing source flag, unknown source
- [x] Cobra `Short`, `Long`, `Example` fields

### 5.12 CLI: export `--type` flag (`internal/cli/export.go`)
- [x] Add `--type desires|invocations` flag (default: `desires`)
- [x] `writeInvocationsJSON()` function
- [x] `writeInvocationsCSV()` function
- [x] CSV columns: id, source, instance_id, host_id, tool_name, is_error, error, cwd, timestamp, metadata
- [x] `--since` filter works for invocations too
- [x] Backward compatible: no `--type` flag behaves exactly as before

### 5.13 CLI: refactor init to plugin-driven (`internal/cli/init_cmd.go`)
- [x] Add `--source NAME` flag
- [x] Keep `--claude-code` as deprecated alias for `--source claude-code`
- [x] Look up source plugin, check if it implements `Installer`
- [x] Delegate to `Installer.InstallHooks()` if available
- [x] Clear error if source doesn't support auto-install
- [x] Refactor `mergeHook()` → `mergeHookEvent(settings, eventName, hook)` (parameterize event name)
- [x] Refactor `hasDPHook()` → `hasDPHookCommand(entries, command)` (parameterize command)

### 5.14 CLI: stats `--invocations` flag (`internal/cli/stats.go`)
- [x] Add `--invocations` flag
- [x] Display invocation stats: total, unique tools, top sources, top tools, time windows
- [x] `--json` support for invocation stats
- [x] Without flag, behavior unchanged (shows desire stats)

### 5.15 Documentation
- [x] Update AGENTS.md: add `internal/source/` and `internal/ingest/` package descriptions
- [x] Update AGENTS.md: document plugin conventions and Installer interface
- [x] Doc comments on all exported symbols in new packages

## Done when

```bash
# Ingest works end-to-end
echo '{"tool_name":"Read","session_id":"s1","tool_use_id":"t1","cwd":"/tmp","hook_event_name":"PostToolUse"}' \
  | ./dp ingest --source claude-code
# exits 0, invocation stored

# Source-specific fields are in metadata, not columns
./dp export --type invocations --format json | grep '"tool_use_id"'
# tool_use_id appears inside metadata JSON, not as top-level field

# Init delegates to plugin
./dp init --source claude-code
# configures hooks via Claude Code Installer

# Stats work for invocations
./dp stats --invocations
# shows invocation summary

# Backward compat: desires unchanged
echo '{"tool_name":"x","error":"e"}' | ./dp record --source test
./dp list   # works as before
./dp export # works as before (defaults to desires)

# All tests pass
make test
```

## Depends on

- Phases 1-4 (all complete)

## Subsequent Phases (Status)

- **Phase 6: Additional source plugins** — Kiro (done), Codex (done), Gemini CLI (dp-286), Cursor (dp-1i5)
- **Phase 7: Record/ingest convergence** — COMPLETE. `dp record` is now a deprecated alias for `dp ingest`.
- **Phase 8: Tool allowlist** — COMPLETE. `track_tools` config key filters in `dp ingest`.
