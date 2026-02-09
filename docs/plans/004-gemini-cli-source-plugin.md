# 004: Gemini CLI Source Plugin

## Summary

Add a `gemini-cli` source plugin for Google's Gemini CLI, enabling dp to ingest tool invocation data from Gemini CLI's `AfterTool` hooks. Gemini CLI uses a hook system architecturally similar to Claude Code — JSON on stdin, event-based matchers, settings.json config — making this a clean integration following the established Kiro/Claude Code patterns.

## Gemini CLI Hook System

Gemini CLI (https://github.com/google-gemini/gemini-cli) provides 11 hook events across four categories: tool hooks (`BeforeTool`, `AfterTool`), agent hooks (`BeforeAgent`, `AfterAgent`), model hooks (`BeforeModel`, `AfterModel`, `BeforeToolSelection`), and lifecycle hooks (`SessionStart`, `SessionEnd`, `Notification`, `PreCompress`).

### Relevant Event: `AfterTool`

Fires after every tool execution. JSON payload on stdin:

```json
{
  "session_id": "abc-123",
  "transcript_path": "/home/user/.gemini/sessions/abc-123.json",
  "cwd": "/path/to/project",
  "hook_event_name": "AfterTool",
  "timestamp": "2026-02-09T10:00:00Z",
  "tool_name": "write_file",
  "tool_input": {
    "file_path": "/tmp/test.go",
    "content": "package main"
  },
  "tool_response": {
    "llmContent": "File written successfully",
    "returnDisplay": "File written successfully",
    "error": ""
  },
  "mcp_context": {}
}
```

### Key Differences from Existing Plugins

| Aspect | Claude Code | Kiro CLI | Gemini CLI |
|--------|-------------|----------|------------|
| Error signaling | Separate `PostToolUseFailure` event + `error` field | Single event; `tool_response.success=false` | Single `AfterTool` event; `tool_response.error` non-empty |
| Session ID | `session_id` field | Not present | `session_id` field |
| Response field | `tool_result` (when present) | `tool_response.{success, result}` | `tool_response.{llmContent, returnDisplay, error}` |
| Config location | `~/.claude/settings.json` | `~/.kiro/agents/*.json` | `~/.gemini/settings.json` |
| Hook structure | Event → `[{matcher, hooks: [{type, command, timeout}]}]` | Event → `[{matcher, command, timeout_ms}]` (flat) | Event → `[{matcher, hooks: [{type, command, timeout, name}]}]` (nested, like Claude) |
| Matcher syntax | Regex (`".*"`) | Glob (`"*"`) | Regex (`".*"`) or exact for lifecycle events |
| Extra fields | `tool_use_id`, `transcript_path`, `hook_event_name`, `permission_mode` | `hook_event_name`, `tool_response` | `transcript_path`, `hook_event_name`, `timestamp`, `tool_response`, `mcp_context` |
| Timeout unit | `timeout` (ms) | `timeout_ms` | `timeout` (ms) |

### Gemini CLI Built-in Tool Names

| Tool | Description |
|------|-------------|
| `read_file` / `read_many_files` | Read files |
| `write_file` | Create and edit files |
| `replace` | Targeted string replacement |
| `edit` | Edit file sections |
| `shell` | Execute shell commands |
| `glob` | File discovery |
| `grep` | Content search |
| `web_search` | Web search |
| `web_fetch` | Fetch URL content |

MCP tools use `mcp__<server>__<tool>` format (e.g., `mcp__postgres__query`).

## Design Decisions

### 1. Stdin model fits perfectly

Gemini CLI hooks pipe JSON to stdin, exactly like Claude Code and Kiro. No adaptation needed — our `Extract(raw []byte)` interface works directly.

### 2. Error detection from `tool_response.error`

Gemini CLI uses a single `AfterTool` event (no separate failure event). Errors are signaled by a non-empty `tool_response.error` string. When present, `Fields.Error` is set to that error string. This enables the dual-write pattern (invocation + desire).

### 3. Session ID available

Unlike Kiro, Gemini CLI provides `session_id` in every hook payload. This maps directly to `Fields.InstanceID`, enabling session-level grouping of invocations.

### 4. Hook structure mirrors Claude Code

Gemini CLI's hook config uses the same nested structure as Claude Code: `{matcher, hooks: [{type, command, timeout}]}`. The installer can reuse a nearly identical pattern to Claude Code's, with the addition of a `name` field.

### 5. Settings file installation

Gemini CLI uses `~/.gemini/settings.json` with a `hooks` top-level key organized by event name. The installer merges into this file, same pattern as Claude Code.

### 6. Transcript path preserved in Extra

The `transcript_path` field provides the path to the full session JSON file. This is valuable metadata preserved in `Fields.Extra` for potential future features.

## Implementation

### Files

- `internal/source/geminicli.go` — Plugin implementation (Source + Installer)
- `internal/source/geminicli_test.go` — Tests

### Field Mapping

| Gemini CLI JSON field | → Fields | Notes |
|-----------------------|----------|-------|
| `tool_name` | `ToolName` | Required |
| `session_id` | `InstanceID` | Maps directly |
| `tool_input` | `ToolInput` | Raw JSON preserved |
| `cwd` | `CWD` | Optional |
| `tool_response.error` (non-empty) | `Error` | Error string from response |
| `hook_event_name` | `Extra` | Gemini-specific |
| `transcript_path` | `Extra` | Gemini-specific |
| `timestamp` | `Extra` | Gemini-specific |
| `tool_response` | `Extra` | Full response preserved |
| `mcp_context` | `Extra` | MCP metadata preserved |

### Known Fields (mapped to universal)

```go
var knownGeminiFields = map[string]bool{
    "tool_name":  true,
    "session_id": true,
    "tool_input": true,
    "cwd":        true,
}
```

### Hook Installation

Default (`dp init --source gemini-cli`):
```
AfterTool → dp record --source gemini-cli
```

With `--track-all`:
```
AfterTool → dp ingest --source gemini-cli
```

Installed to: `~/.gemini/settings.json`.

### Example Settings Output

```json
{
  "hooks": {
    "AfterTool": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "dp ingest --source gemini-cli",
            "name": "dp-ingest",
            "timeout": 5000
          }
        ]
      }
    ]
  }
}
```

### Installer Details

The installer follows the Claude Code pattern (nested hook entries) since Gemini CLI uses the same nested structure:

```go
type geminiHookEntry struct {
    Matcher string            `json:"matcher"`
    Hooks   []geminiHookInner `json:"hooks"`
}

type geminiHookInner struct {
    Type    string `json:"type"`
    Command string `json:"command"`
    Name    string `json:"name,omitempty"`
    Timeout int    `json:"timeout"`
}
```

Config path: `~/.gemini/settings.json` (same on all platforms — Gemini CLI does not use XDG conventions for the primary settings file).

### CLI Integration

The `defaultConfigDir` function in `sources.go` needs a new case:
```go
case "gemini-cli":
    return filepath.Join(home, ".gemini")
```

## Usage

```bash
# Install hooks
dp init --source gemini-cli

# Install with all-invocation tracking
dp init --source gemini-cli --track-all

# Manual ingestion
echo '{"tool_name":"write_file","session_id":"abc","cwd":"/tmp","tool_response":{"error":""}}' | dp ingest --source gemini-cli

# Check installation status
dp sources
```

## Testing Plan

Following the established test pattern from kiro_test.go and claudecode_test.go:

1. **Name/Description** — Verify `"gemini-cli"` name and non-empty description
2. **Extract: full payload** — All universal fields mapped correctly, extras preserved
3. **Extract: missing tool_name** — Returns error
4. **Extract: empty tool_name** — Returns error
5. **Extract: error detection** — `tool_response.error` non-empty sets `Fields.Error`
6. **Extract: success case** — Empty `tool_response.error` leaves `Fields.Error` empty
7. **Extract: session_id mapping** — Maps to `Fields.InstanceID`
8. **Extract: extra fields** — Unknown fields preserved in Extra, known fields excluded
9. **Install: creates settings file** — Fresh install in temp dir
10. **Install: merges existing settings** — Preserves existing hooks
11. **Install: idempotent** — Running twice doesn't duplicate entries
12. **IsInstalled: detects current command** — Finds `dp ingest --source gemini-cli`
13. **IsInstalled: detects legacy command** — Finds `dp record --source gemini-cli`
14. **IsInstalled: returns false when absent** — No hooks configured
