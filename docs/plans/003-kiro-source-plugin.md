# 003: Kiro CLI Source Plugin

## Summary

Add a `kiro` source plugin for Kiro CLI (Amazon/AWS), enabling dp to ingest tool invocation data from Kiro's `postToolUse` hooks. Kiro CLI's hook system is architecturally near-identical to Claude Code's, making this a straightforward integration.

## Kiro CLI Hook System

Kiro CLI (https://kiro.dev) provides five hook events: `agentSpawn`, `userPromptSubmit`, `preToolUse`, `postToolUse`, and `stop`. Hooks receive JSON via **stdin** and are configured in agent config files.

### Relevant Event: `postToolUse`

Fires after every tool execution. JSON payload:

```json
{
  "hook_event_name": "postToolUse",
  "cwd": "/path/to/project",
  "tool_name": "write",
  "tool_input": {
    "file_path": "/tmp/test.go",
    "content": "package main"
  },
  "tool_response": {
    "success": true,
    "result": ["File written successfully"]
  }
}
```

### Key Differences from Claude Code

| Aspect | Claude Code | Kiro CLI |
|--------|-------------|----------|
| Error signaling | Separate `PostToolUseFailure` event + `error` field | Single `postToolUse` event; `tool_response.success=false` |
| Session ID | `session_id` field in payload | Not present in hook payload |
| Response field | `tool_result` (when present) | `tool_response` with `{success, result}` structure |
| Config location | `~/.claude/settings.json` | `~/.kiro/agents/*.json` (macOS: `~/.kiro/`, Linux: `~/.config/kiro/`) |
| Hook structure | Event → array of `{matcher, hooks: [{type, command, timeout}]}` | Event → array of `{matcher, command, timeout_ms}` (flat) |
| Matcher syntax | Regex (`".*"`) | Glob/keyword (`"*"`, `"@builtin"`, `"@git"`) |
| Extra features | — | `cache_ttl_seconds` for deduplication |

### Kiro Built-in Tool Names

| Tool | Description |
|------|-------------|
| `read` / `fs_read` | Read files/folders/images |
| `write` / `fs_write` | Create and edit files |
| `shell` / `execute_bash` | Execute bash commands |
| `aws` / `use_aws` | AWS CLI calls |
| `glob` | File discovery |
| `grep` | Content search |
| `web_search` | Web search |
| `web_fetch` | Fetch URL content |
| `delegate` | Background agent delegation |
| `knowledge` | Cross-session storage |

MCP tools use `@server/tool` format (e.g., `@postgres/query`).

## Design Decisions

### 1. Stdin model fits perfectly

Kiro's hooks pipe JSON to stdin, exactly like Claude Code. No adaptation needed — our `Extract(raw []byte)` interface works directly.

### 2. Error detection from `tool_response.success`

Since Kiro has no separate failure event, the plugin checks `tool_response.success` during extraction. When `false`, `Fields.Error` is set to `"tool call failed"`. This enables the dual-write pattern (invocation + desire) without requiring a separate hook event.

### 3. No InstanceID

Kiro payloads do not include a session identifier. `Fields.InstanceID` remains empty. This means invocations cannot be grouped by session without external correlation (e.g., by `cwd` + timestamp windows). This is acceptable for initial integration.

### 4. Agent config file installation

Unlike Claude Code's monolithic `settings.json`, Kiro uses per-agent config files in `~/.kiro/agents/` (or `~/.config/kiro/agents/` on Linux). The installer creates `dp-hooks.json` as a dedicated agent config, avoiding collision with user-defined agents.

### 5. Flat hook entry structure

Kiro hook entries use a flat `{matcher, command, timeout_ms}` format rather than Claude Code's nested `{matcher, hooks: [{type, command, timeout}]}`. The plugin types reflect this difference.

## Implementation

### Files

- `internal/source/kiro.go` — Plugin implementation (Source + Installer)
- `internal/source/kiro_test.go` — Tests

### Field Mapping

| Kiro JSON field | → Fields | Notes |
|-----------------|----------|-------|
| `tool_name` | `ToolName` | Required |
| `tool_input` | `ToolInput` | Raw JSON preserved |
| `cwd` | `CWD` | Optional |
| `tool_response.success=false` | `Error` | Set to "tool call failed" |
| `hook_event_name` | `Extra` | Kiro-specific |
| `tool_response` | `Extra` | Full response preserved |

### Hook Installation

Default (`dp init --source kiro`):
```
postToolUse → dp record --source kiro
```

With `--track-all`:
```
postToolUse → dp ingest --source kiro
```

Installed to: `~/.kiro/agents/dp-hooks.json` (macOS) or `~/.config/kiro/agents/dp-hooks.json` (Linux).

### Example Agent Config Output

```json
{
  "hooks": {
    "postToolUse": [
      {
        "matcher": "*",
        "command": "dp record --source kiro",
        "timeout_ms": 5000
      }
    ]
  }
}
```

## Usage

```bash
# Install hooks
dp init --source kiro

# Install with all-invocation tracking
dp init --source kiro --track-all

# Manual ingestion
echo '{"tool_name":"write","cwd":"/tmp","tool_response":{"success":true}}' | dp ingest --source kiro

# Check installation status
dp sources
```
