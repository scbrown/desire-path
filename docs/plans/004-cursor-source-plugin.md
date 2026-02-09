# 004: Cursor Source Plugin

## Summary

Add a `cursor` source plugin for Cursor IDE, enabling dp to ingest tool invocation data from Cursor's hook system. Cursor (a VS Code fork) provides a hooks mechanism architecturally near-identical to Claude Code's — shell commands receive JSON via stdin on tool call events. The existing stdin JSON plugin model fits without adaptation.

## Cursor Hook System

Cursor (https://cursor.com) supports hooks as of version 1.7 (October 2025). Hooks are configured in `.cursor/hooks.json` (project-level), `~/.cursor/hooks.json` (user-level), or `/etc/cursor/hooks.json` (enterprise). Hook commands receive JSON payloads via **stdin**, exactly like Claude Code and Kiro.

### Relevant Hook Events

| Hook Event | Trigger | Tool Call Data? |
|---|---|---|
| `preToolUse` | Before any tool runs | tool_name, tool_input, tool_use_id |
| `postToolUse` | After tool completes | tool_name, tool_input, tool_output, duration |
| `postToolUseFailure` | Tool fails or denied | tool_name, error_message, failure_type |
| `afterFileEdit` | After file modification | file_path, edits (old_string/new_string) |
| `afterShellExecution` | After terminal command | command, output, duration |
| `beforeMCPExecution` | Before MCP tool call | server, tool_name, tool_input |
| `afterMCPExecution` | After MCP tool call | tool_name, tool_input, result_json, duration |

**Common metadata on all hooks:** `conversation_id`, `generation_id`, `model`, `cursor_version`, `workspace_roots`, `user_email`, `transcript_path`.

### Primary Event: `postToolUse`

Fires after every tool execution. Estimated JSON payload:

```json
{
  "hook_event_name": "postToolUse",
  "tool_name": "edit_file",
  "tool_input": {
    "file_path": "/home/user/project/main.go",
    "old_string": "func old()",
    "new_string": "func new()"
  },
  "tool_output": "File edited successfully",
  "tool_use_id": "toolu_abc123",
  "conversation_id": "conv-xyz",
  "generation_id": "gen-456",
  "model": "cursor-fast",
  "cursor_version": "2.2.44",
  "cwd": "/home/user/project",
  "duration": 150,
  "transcript_path": "/tmp/cursor-transcript-abc.json"
}
```

### Secondary Event: `postToolUseFailure`

Fires when a tool fails or is denied:

```json
{
  "hook_event_name": "postToolUseFailure",
  "tool_name": "edit_file",
  "tool_input": { "file_path": "/etc/passwd" },
  "error_message": "Permission denied",
  "failure_type": "denied",
  "conversation_id": "conv-xyz",
  "cwd": "/home/user/project"
}
```

### Key Differences from Claude Code and Kiro

| Aspect | Claude Code | Kiro CLI | Cursor |
|---|---|---|---|
| Error signaling | Separate `PostToolUseFailure` event + `error` field | Single `postToolUse`; `tool_response.success=false` | Separate `postToolUseFailure` event + `error_message` field |
| Session ID | `session_id` | Not present | `conversation_id` |
| Config location | `~/.claude/settings.json` | `~/.kiro/agents/*.json` | `~/.cursor/hooks.json` or `.cursor/hooks.json` |
| Hook structure | Nested `{matcher, hooks: [{type, command, timeout}]}` | Flat `{matcher, command, timeout_ms}` | `{command, event}` (simpler) |
| Output data | Not in hook payload | `tool_response` object | `tool_output` field + `duration` |
| Extra context | `transcript_path`, `permission_mode` | — | `conversation_id`, `model`, `cursor_version`, `transcript_path` |

### Cursor Built-in Tool Names

Cursor 2.0+ agent mode provides ~10 internal tools:

| Tool | Description |
|---|---|
| `edit_file` | Edit files with old/new string replacement |
| `read_file` | Read file contents |
| `create_file` | Create new files |
| `delete_file` | Delete files |
| `run_terminal_command` | Execute shell commands |
| `search_files` | Semantic search across codebase |
| `list_directory` | List directory contents |
| `codebase_search` | Code search |
| `grep_search` | Content grep |
| `web_search` | Web search |

MCP tools appear as `mcp_<server>_<tool>` format.

## Design Decisions

### 1. Stdin model fits perfectly

Cursor's hooks pipe JSON to stdin via shell commands, exactly like Claude Code and Kiro. No adaptation needed — the `Extract(raw []byte)` interface works directly.

**Verdict: The existing plugin model works. No adapter pattern needed.**

### 2. Dual hook events for success/failure (like Claude Code)

Cursor uses separate `postToolUse` and `postToolUseFailure` events, mirroring Claude Code's pattern exactly. The plugin maps `error_message` → `Fields.Error` from failure events, enabling the existing dual-write pattern (invocation + desire) without changes.

### 3. conversation_id → InstanceID

Cursor provides `conversation_id` in hook payloads, which maps naturally to `Fields.InstanceID`. This enables grouping invocations by conversation session — an improvement over Kiro which lacks session IDs.

### 4. Rich Extra fields

Cursor hooks provide useful metadata not in the universal Fields: `model`, `cursor_version`, `generation_id`, `duration`, `transcript_path`, `tool_output`. These go into `Fields.Extra` for downstream analysis.

### 5. Hooks config file format

Cursor's hooks.json uses a simpler format than Claude Code:

```json
{
  "hooks": {
    "postToolUse": {
      "command": "dp ingest --source cursor",
      "event": "postToolUse"
    },
    "postToolUseFailure": {
      "command": "dp ingest --source cursor",
      "event": "postToolUseFailure"
    }
  }
}
```

The Installer creates/merges into `~/.cursor/hooks.json` (user-level) to capture across all projects.

### 6. Config directory detection

| OS | Config Dir |
|---|---|
| Linux | `~/.config/Cursor/` or `~/.cursor/` |
| macOS | `~/Library/Application Support/Cursor/` or `~/.cursor/` |
| Windows | `%APPDATA%\Cursor\` |

The hooks file is at `~/.cursor/hooks.json` (cross-platform user-level). The Installer targets this location.

## Implementation

### Files

- `internal/source/cursor.go` — Plugin implementation (Source + Installer)
- `internal/source/cursor_test.go` — Tests

### Field Mapping

| Cursor JSON field | → Fields | Notes |
|---|---|---|
| `tool_name` | `ToolName` | Required |
| `tool_input` | `ToolInput` | Raw JSON preserved |
| `cwd` | `CWD` | Optional |
| `conversation_id` | `InstanceID` | Session grouping |
| `error_message` | `Error` | From `postToolUseFailure` events |
| `hook_event_name` | `Extra` | Cursor-specific |
| `tool_output` | `Extra` | Success output preserved |
| `model` | `Extra` | Which model ran the tool |
| `cursor_version` | `Extra` | Version tracking |
| `generation_id` | `Extra` | Per-generation ID |
| `duration` | `Extra` | Execution time |
| `transcript_path` | `Extra` | Full transcript location |
| `failure_type` | `Extra` | Error categorization |

### Hook Installation

Default (`dp init --source cursor`):
```
postToolUse         → dp ingest --source cursor
postToolUseFailure  → dp ingest --source cursor
```

Installed to: `~/.cursor/hooks.json`.

### Example hooks.json Output

```json
{
  "hooks": {
    "postToolUse": {
      "command": "dp ingest --source cursor",
      "event": "postToolUse"
    },
    "postToolUseFailure": {
      "command": "dp ingest --source cursor",
      "event": "postToolUseFailure"
    }
  }
}
```

## Alternative Data Source: SQLite Database

For batch/historical ingestion (not real-time hooks), Cursor stores all conversation and tool call data in local SQLite databases:

| OS | Path |
|---|---|
| Linux | `~/.config/Cursor/User/workspaceStorage/<hash>/state.vscdb` |
| macOS | `~/Library/Application Support/Cursor/User/workspaceStorage/<hash>/state.vscdb` |
| Windows | `%APPDATA%\Cursor\User\workspaceStorage\<hash>\state.vscdb` |

Schema: `cursorDiskKV` table with keys like `composerData:<id>` and `bubbleId:<composerId>:<bubbleId>`. Each bubble (message) contains a `toolFormerData` field with structured tool call data.

This could be a future enhancement for backfilling historical data, but hooks are the recommended primary mechanism.

### Existing open-source parsers

| Project | Language | Notes |
|---|---|---|
| cursor-history (S2thend) | Node.js | Reads `cursorDiskKV`, extracts tool calls |
| cursor-helper (lucifer1004) | Rust | Exports thinking blocks + tool calls |
| ai-data-extraction (0xSero) | Python | Multi-version schema support |

## Risks and Caveats

1. **Hook payload format is not fully documented.** The exact JSON schema for Cursor hook events is based on community documentation and blog posts. The implementation should be defensive about missing fields.

2. **Schema evolution.** Cursor's hook payload has changed across versions (1.7 → 2.0 → 2.2). The plugin should handle missing fields gracefully (optional fields with zero-value defaults).

3. **hooks.json format may differ from docs.** The exact structure of Cursor's hooks.json needs validation during implementation. The format shown in Cursor's official docs and GitButler's deep dive differ slightly. Implementation should verify against the actual Cursor version.

4. **No `track_tools` filtering concern.** Since Cursor hooks fire for all tool calls by default, the existing `track_tools` allowlist in dp's config handles filtering.

## Feasibility Assessment

**Overall: HIGH feasibility.** Cursor's hook system is architecturally identical to Claude Code's. The implementation follows the exact same pattern as the existing `claudecode.go` plugin:

- Source interface: `Extract(raw []byte) (*Fields, error)` — parse Cursor's JSON, map to universal fields
- Installer interface: `Install(opts)` / `IsInstalled(configDir)` — create/check `~/.cursor/hooks.json`
- Self-registration via `init()` function

Estimated effort: ~200 lines of Go (source + installer), ~150 lines of tests. One-to-two day implementation.

## Usage

```bash
# Install hooks
dp init --source cursor

# Manual ingestion (for testing)
echo '{"tool_name":"edit_file","cwd":"/tmp","conversation_id":"conv-1"}' | dp ingest --source cursor

# Check installation status
dp sources

# Query Cursor data
dp list --source cursor
dp stats --source cursor
```
