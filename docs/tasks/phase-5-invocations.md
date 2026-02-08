# Phase 5: Invocation Tracking

Adds tracking of all tool invocations (not just failures), with a plugin system for multi-tool support. See `docs/plans/002-invocation-tracking.md` for full architecture.

## Tasks

### 5.1 Source plugin interface (`internal/source/source.go`)
- [ ] `Source` interface: `Name() string` + `Extract(raw []byte) (Fields, error)`
- [ ] `Fields` struct with universal fields only: ToolName, InstanceID, ToolInput, CWD, Error, Extra map
- [ ] `Installer` optional interface: `InstallHooks(configDir string) error`
- [ ] Registry: `Register(s Source)` / `Get(name string) (Source, bool)` / `Names() []string`
- [ ] Panic or error on duplicate registration

### 5.2 Claude Code plugin (`internal/source/claudecode.go`)
- [ ] Implement `Source` interface
- [ ] `Name()` returns `"claude-code"`
- [ ] `Extract()` parses Claude Code hook JSON from raw bytes
- [ ] Maps `session_id` → `InstanceID`, `tool_name` → `ToolName`, `cwd` → `CWD`, `error` → `Error`
- [ ] Extracts `tool_input` into `ToolInput`
- [ ] Puts `tool_use_id`, `transcript_path`, `hook_event_name`, `permission_mode` into `Extra`
- [ ] Collects any unknown keys into `Extra`
- [ ] Implements `Installer` — sets up PostToolUse + PostToolUseFailure hooks in `~/.claude/settings.json`
- [ ] Self-registers via `init()`

### 5.3 Source plugin tests (`internal/source/source_test.go`)
- [ ] Registry: register, get, names, duplicate panics
- [ ] Claude Code Extract: full payload, minimal payload, missing tool_name error
- [ ] Claude Code Extract: source-specific fields land in Extra (not top-level)
- [ ] Claude Code Installer: creates hooks in temp settings file, idempotent re-run

### 5.4 Invocation model (`internal/model/model.go`)
- [ ] Add `Invocation` struct: ID, Source, InstanceID, HostID, ToolName, IsError, Error, CWD, Timestamp, Metadata
- [ ] JSON tags with omitempty on optional fields
- [ ] No source-specific fields (hook_event, transcript_path, tool_use_id go in Metadata)

### 5.5 Store interface changes (`internal/store/store.go`)
- [ ] Add `RecordInvocation(ctx, Invocation) error`
- [ ] Add `ListInvocations(ctx, InvocationOpts) ([]Invocation, error)`
- [ ] Add `InvocationStats(ctx) (InvocationStats, error)`
- [ ] `InvocationOpts` struct: Since, Source, InstanceID, ToolName, ErrorsOnly, Limit
- [ ] `InvocationStats` struct: totals, unique tools, top sources, top tools, time windows, date range

### 5.6 SQLite migration v2 (`internal/store/sqlite.go`)
- [ ] Schema version 2 migration
- [ ] `invocations` table with universal columns only: id, source, instance_id, host_id, tool_name, is_error, error, cwd, timestamp, metadata
- [ ] Indexes on source, instance_id, tool_name, timestamp, is_error
- [ ] `RecordInvocation` implementation (ExecContext INSERT)
- [ ] `ListInvocations` implementation (dynamic WHERE clauses)
- [ ] `InvocationStats` implementation (aggregate queries)
- [ ] `boolToInt()` helper for is_error

### 5.7 Update fakeStore for test compilation
- [ ] Add stub `RecordInvocation`, `ListInvocations`, `InvocationStats` to fakeStore in `record_test.go`
- [ ] Existing tests still pass with no behavior changes

### 5.8 Store tests for invocations
- [ ] RecordInvocation: insert and retrieve round-trip
- [ ] ListInvocations: filter by source, instance_id, tool_name, errors_only, since, limit
- [ ] InvocationStats: correct totals, top tools, time windows
- [ ] Metadata JSON preserved correctly through insert/retrieve cycle
- [ ] Migration v2 applied cleanly on fresh DB and on existing v1 DB

### 5.9 Ingest package (`internal/ingest/`)
- [ ] `Ingest(ctx, store, reader, sourceName) (Invocation, error)`
- [ ] Reads raw bytes from reader
- [ ] Looks up source plugin by name, returns error if not found
- [ ] Calls `plugin.Extract(raw)` to get Fields
- [ ] Generates UUID, timestamp, host_id
- [ ] Derives `is_error` from `Fields.Error != ""`
- [ ] Marshals `Fields.Extra` → `Invocation.Metadata`
- [ ] Calls `store.RecordInvocation()`

### 5.10 Ingest tests (`internal/ingest/ingest_test.go`)
- [ ] Full Claude Code payload → correct Invocation fields
- [ ] Source-specific fields in Extra end up in Metadata JSON
- [ ] Missing tool_name → error
- [ ] Unknown source name → error
- [ ] UUID and timestamp auto-generated

### 5.11 CLI: `dp ingest` command (`internal/cli/ingest.go`)
- [ ] `--source` flag (required)
- [ ] Reads stdin, calls `ingest.Ingest()`
- [ ] Supports `--json` global flag for output
- [ ] Error messages for missing source flag, unknown source
- [ ] Cobra `Short`, `Long`, `Example` fields

### 5.12 CLI: export `--type` flag (`internal/cli/export.go`)
- [ ] Add `--type desires|invocations` flag (default: `desires`)
- [ ] `writeInvocationsJSON()` function
- [ ] `writeInvocationsCSV()` function
- [ ] CSV columns: id, source, instance_id, host_id, tool_name, is_error, error, cwd, timestamp, metadata
- [ ] `--since` filter works for invocations too
- [ ] Backward compatible: no `--type` flag behaves exactly as before

### 5.13 CLI: refactor init to plugin-driven (`internal/cli/init_cmd.go`)
- [ ] Add `--source NAME` flag
- [ ] Keep `--claude-code` as deprecated alias for `--source claude-code`
- [ ] Look up source plugin, check if it implements `Installer`
- [ ] Delegate to `Installer.InstallHooks()` if available
- [ ] Clear error if source doesn't support auto-install
- [ ] Refactor `mergeHook()` → `mergeHookEvent(settings, eventName, hook)` (parameterize event name)
- [ ] Refactor `hasDPHook()` → `hasDPHookCommand(entries, command)` (parameterize command)

### 5.14 CLI: stats `--invocations` flag (`internal/cli/stats.go`)
- [ ] Add `--invocations` flag
- [ ] Display invocation stats: total, unique tools, top sources, top tools, time windows
- [ ] `--json` support for invocation stats
- [ ] Without flag, behavior unchanged (shows desire stats)

### 5.15 Documentation
- [ ] Update AGENTS.md: add `internal/source/` and `internal/ingest/` package descriptions
- [ ] Update AGENTS.md: document plugin conventions and Installer interface
- [ ] Doc comments on all exported symbols in new packages

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

## Blocks

- Phase 6: Additional source plugins (Gemini CLI, Kiro CLI, OpenCode)
- Phase 7: Record/ingest convergence
- Phase 8: Tool allowlist
