# Claude Code Integration

Claude Code is Anthropic's official CLI for Claude. It provides a hook system that allows external commands to run on various events. dp integrates with Claude Code by installing hooks that capture tool call failures (and optionally all tool calls) for analysis.

## Quick Setup

```bash
dp init --source claude-code
```

This command updates `~/.claude/settings.json` to add a `PostToolUseFailure` hook. It's idempotent—safe to run multiple times.

## What Are Claude Code Hooks?

Claude Code fires hooks at specific lifecycle events. The most relevant for dp:

- **PostToolUseFailure**: Fires when a tool call fails (tool not found, invalid input, execution error)
- **PostToolUse**: Fires after every tool call, whether it succeeds or fails

Hooks receive a JSON payload describing the event. They execute asynchronously—Claude Code doesn't wait for the hook to complete, so dp processing never slows down your session.

## Hook Configuration

### Default Setup (Failures Only)

`dp init --source claude-code` writes this to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "PostToolUseFailure": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "dp record --source claude-code",
            "timeout": 5000
          }
        ]
      }
    ]
  }
}
```

The `matcher: ".*"` means "match all sessions." Every time a tool call fails, Claude Code pipes the failure JSON to `dp record --source claude-code` via stdin. dp parses the JSON, extracts fields, and writes a desire record to `~/.dp/desires.db`.

### Full Tracking (Successes + Failures)

To track *all* tool invocations (not just failures), enable full tracking:

```bash
dp init --source claude-code --track-all
```

This adds two additional hooks:

```json
{
  "hooks": {
    "PostToolUseFailure": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "dp record --source claude-code",
            "timeout": 5000
          },
          {
            "type": "command",
            "command": "dp ingest --source claude-code",
            "timeout": 5000
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "dp ingest --source claude-code",
            "timeout": 5000
          }
        ]
      }
    ]
  }
}
```

Now:
- `PostToolUseFailure` runs both `dp record` (for desires) and `dp ingest` (for invocations)
- `PostToolUse` runs `dp ingest` for all successful calls

This generates more data—every tool call fires a hook—so only enable it if you need invocation-level analytics (success rates, call frequency, session analysis, etc.).

## Hook Payload Format

Claude Code passes a JSON object on stdin. Example for a failed tool call:

```json
{
  "session_id": "a1b2c3d4-5678-90ab-cdef-1234567890ab",
  "hook_event_name": "PostToolUseFailure",
  "tool_name": "read_file",
  "tool_use_id": "toolu_01ABC123",
  "tool_input": {
    "file_path": "/tmp/nonexistent.txt"
  },
  "error": "File not found: /tmp/nonexistent.txt",
  "cwd": "/home/user/project",
  "transcript_path": "/home/user/.claude/transcripts/2026-02-09-session.json",
  "permission_mode": "normal"
}
```

## Field Mapping

dp's `claude-code` plugin maps these fields to universal `Fields`:

| Claude Code Field | Universal Field | Notes |
|------------------|-----------------|-------|
| `tool_name` | `ToolName` | Required |
| `session_id` | `InstanceID` | Session identifier |
| `tool_input` | `ToolInput` | Preserved as raw JSON |
| `cwd` | `CWD` | Working directory |
| `error` | `Error` | Error message (only present on failures) |

Everything else goes into `Extra`:

- `tool_use_id`: Claude's internal ID for the tool call
- `transcript_path`: Path to the session transcript file
- `hook_event_name`: Which hook fired (`PostToolUseFailure` or `PostToolUse`)
- `permission_mode`: Permission level (`normal`, `strict`, etc.)

These fields are stored in the `metadata` column as JSON, available for queries but not indexed.

## Commands Used by Hooks

### `dp record --source claude-code`

Records a desire (failed tool call). Reads JSON from stdin, extracts fields using the `claude-code` plugin, generates a UUID and timestamp, and writes to the `desires` table.

Example manual invocation:

```bash
echo '{"tool_name":"read_file","error":"unknown tool","session_id":"test","cwd":"/tmp"}' \
  | dp record --source claude-code
```

### `dp ingest --source claude-code`

Records an invocation (any tool call, success or failure). Reads JSON from stdin, extracts fields, sets `is_error` based on presence of `error` field, and writes to the `invocations` table.

Example manual invocation:

```bash
echo '{"tool_name":"Read","session_id":"test","cwd":"/tmp"}' \
  | dp ingest --source claude-code
```

## Idempotency

`dp init --source claude-code` is idempotent. If hooks already exist, it won't add duplicates. It merges the new hooks into the existing `hooks` config, preserving any other hooks you've configured.

Run it multiple times safely:

```bash
dp init --source claude-code
dp init --source claude-code
dp init --source claude-code
```

The second and third runs do nothing (hook already present).

Switching from default to full tracking:

```bash
dp init --source claude-code              # adds PostToolUseFailure → dp record
dp init --source claude-code --track-all  # adds PostToolUse/PostToolUseFailure → dp ingest
```

The second command adds the `ingest` hooks without removing the `record` hook. This is safe—both commands write to different tables (`desires` vs `invocations`).

## Hook Execution Details

- **Timeout**: 5 seconds (configurable in the JSON). If dp takes longer, Claude Code kills the process.
- **Stdin/Stdout**: Hook receives JSON on stdin. Stdout/stderr are discarded (not shown to the user).
- **Exit Code**: Ignored. Hook failures don't affect Claude Code.
- **Async**: Hook runs in the background. Claude Code continues immediately.

Typical execution time: ~5-10ms for `dp record`, ~10-20ms for `dp ingest`.

## Troubleshooting

### Hooks Not Firing

Check that `dp` is in your `PATH`:

```bash
which dp
```

If it's not found, Claude Code can't execute the hook. Install dp to a location in `PATH` (like `$HOME/go/bin` or `/usr/local/bin`).

Verify the hooks are installed:

```bash
cat ~/.claude/settings.json | jq '.hooks'
```

You should see `PostToolUseFailure` with a `dp record` command.

### No Desires Being Recorded

Manually trigger a failure and check the database:

```bash
echo '{"tool_name":"test_tool","error":"test error","session_id":"manual","cwd":"/tmp"}' \
  | dp record --source claude-code

dp list --limit 1
```

If the desire appears, hooks are working. If not, check that `~/.dp/desires.db` is writable.

Enable verbose logging (if dp supported it—currently it doesn't) or use `strace` to debug:

```bash
strace -e trace=open,write dp record --source claude-code < payload.json
```

### Database Locked Errors

SQLite uses WAL mode for concurrent reads/writes, but if another process holds a write lock (like a long-running transaction), writes may block briefly. This is rare—most writes complete in milliseconds.

If you see "database is locked" errors frequently:

1. Check for long-running `dp` commands (like `dp export` on a huge database)
2. Verify no other process is holding the database open
3. Check disk I/O (slow disks can cause lock contention)

SQLite's busy timeout is set to 5 seconds—writes retry automatically during that window.

## Data Storage

### Desires Table

Schema:

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
```

Each `dp record` writes one row.

### Invocations Table

Schema:

```sql
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
```

Each `dp ingest` writes one row. `is_error` is 1 if `error` field was present in the payload, 0 otherwise.

## Query Examples

List all Claude Code desires:

```bash
dp list --source claude-code
```

View aggregated paths:

```bash
dp paths
```

Inspect a specific tool name:

```bash
dp inspect read_file
```

Show invocation stats (requires `--track-all`):

```bash
dp stats --invocations
```

List all invocations from a session:

```bash
sqlite3 ~/.dp/desires.db "SELECT tool_name, is_error, timestamp FROM invocations WHERE instance_id = 'session-id-here' ORDER BY timestamp;"
```

## Performance Notes

- Desire recording: ~5ms per record (dominated by SQLite write + fsync)
- Invocation ingestion: ~10ms per record (slightly larger payloads)
- Database size: ~1KB per desire, ~1.5KB per invocation (including JSON metadata)
- After 10,000 desires: ~10MB database
- After 100,000 invocations: ~150MB database

WAL mode keeps reads fast even during writes. Queries are instant up to ~1M records.

## Next Steps

- [Configuration](../configuration.md): Customize database path, known tools, etc.
- [Command Reference](../commands/README.md): Explore all dp commands
- [Writing a Plugin](./writing-plugins.md): Build a plugin for another AI tool
- [Architecture](../architecture.md): Understand dp's internals
