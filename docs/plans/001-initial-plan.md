# 001 - Initial Plan: Desire Path (dp)

## Context

AI tools like Claude Code sometimes attempt tool calls that fail - hallucinated tool names, wrong parameters, nonexistent commands. These failures are signal, not noise: they reveal what the AI *wanted* to do. Like worn trails across a lawn, they show where paths should be built.

**Desire Path** (`dp`) is a Go CLI + library that collects, analyzes, and surfaces patterns from failed AI tool calls, enabling developers to implement features or aliases so future similar attempts succeed. It's designed to be generic enough for any GenAI-first tool or CLI binary, with first-class Claude Code integration via the `PostToolUseFailure` hook.

## Architecture

### Core Data Model

**Desire** - A single failed tool call:

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Unique identifier |
| tool_name | string | What was attempted (e.g., `"read_file"`, `"mcp__memory__search"`) |
| tool_input | JSON | Parameters passed to the tool |
| error | string | Failure message |
| source | string | Which AI tool (`claude-code`, `cursor`, `copilot`, etc.) |
| session_id | string | Session identifier |
| cwd | string | Working directory at time of failure |
| timestamp | datetime | When it occurred |
| metadata | JSON | Extensible extra data (model, user, etc.) |

**Path** - An aggregated pattern of repeated desires:

| Field | Type | Description |
|-------|------|-------------|
| id | UUID | Unique identifier |
| pattern | string | Normalized tool name pattern |
| count | int | Total occurrences |
| first_seen | datetime | Earliest occurrence |
| last_seen | datetime | Most recent occurrence |
| alias_to | string | Optional mapping to a real tool/command |

### Storage

- SQLite via `modernc.org/sqlite` (pure Go, no CGo)
- Default location: `~/.dp/desires.db`
- Configurable via `--db` flag or `DESIRE_PATH_DB` env var

### Integration Protocol

**Stdin JSON mode (primary):**
```bash
echo '{"tool_name":"read_file","error":"unknown tool"}' | dp record --source my-tool
```

**Claude Code hook:**
```json
{
  "hooks": {
    "PostToolUseFailure": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "dp record --source claude-code",
            "async": true
          }
        ]
      }
    ]
  }
}
```

The `PostToolUseFailure` hook provides on stdin:
```json
{
  "session_id": "abc123",
  "hook_event_name": "PostToolUseFailure",
  "tool_name": "Bash",
  "tool_input": { "command": "..." },
  "tool_use_id": "toolu_01...",
  "error": "Command exited with non-zero status code 1",
  "cwd": "/path/to/project",
  "transcript_path": "...",
  "permission_mode": "default"
}
```

## CLI Commands

All commands support `--json` for machine-readable output and `--db` to override the database path.

```
dp record [--source NAME]            Record a desire from stdin JSON
dp list [--since] [--source] [--tool] [--limit]  List recent desires
dp paths [--top 20] [--since]        Show aggregated paths ranked by frequency
dp inspect <pattern> [--since] [--top]  Detailed view of a specific path
dp suggest <tool-name> [--known] [--threshold] [--top]  Suggest tool mappings via similarity
dp alias <from> <to>                 Map a hallucinated name to a real tool
dp alias --delete <from>             Remove an alias
dp aliases                           List all configured aliases
dp export [--format json|csv] [--since]  Export raw data
dp stats                             Summary statistics
dp init --claude-code                Set up Claude Code hook integration
dp config [key] [value]              View/set configuration
```

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `modernc.org/sqlite` | Pure Go SQLite (no CGo) |
| `github.com/google/uuid` | UUID generation |
| `golang.org/x/term` | Terminal detection and width |

Note: String similarity for suggestions uses an internal Levenshtein implementation in `internal/analyze/`.

## Configuration

Settings stored in `~/.dp/config.json`, managed via `dp config`. The config package (`internal/config/`) handles load/save with JSON serialization. Valid keys: `db_path`, `default_source`, `known_tools`, `default_format`. Config values are applied in `PersistentPreRun` and can be overridden by flags.

## Output Formatting

- **Table output**: All tabular commands use `internal/cli.Table`, a wrapper around `text/tabwriter` that auto-detects TTY for color (ANSI bold headers) and terminal width.
- **JSON output**: The `--json` global flag on the root command enables machine-readable JSON output on all commands. When `default_format` is set to `json` in config, JSON is the default.
- **TTY detection**: Uses `golang.org/x/term` to detect terminal and query width. Non-TTY output gets plain text at 80-column default width.

## MVP Phases

See `docs/tasks/` for detailed task breakdowns:
- [Phase 1: Core](../tasks/phase-1-core.md) - Types, storage, record command
- [Phase 2: Reporting](../tasks/phase-2-reporting.md) - list, paths, stats, export commands
- [Phase 3: Suggestions & Aliases](../tasks/phase-3-suggestions.md) - suggest, alias, init commands
- [Phase 4: Polish](../tasks/phase-4-polish.md) - Output formatting, --json flag, config, tests
