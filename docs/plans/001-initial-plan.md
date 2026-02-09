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

```
dp record [--source NAME]       Record a desire from stdin JSON
dp list [--since 7d] [--source] List recent desires
dp paths [--top 20]             Show aggregated paths ranked by frequency
dp inspect <path-pattern>       Detailed view of a specific path
dp suggest <tool-name>          Suggest existing tool mappings via similarity
dp alias <from> <to>            Map a hallucinated name to a real tool
dp aliases                      List all configured aliases
dp export [--format json|csv]   Export raw data
dp stats                        Summary statistics
dp init [--claude-code]         Set up integration (e.g., write Claude Code hook config)
dp config [key] [value]         Manage configuration
```

## Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `modernc.org/sqlite` | Pure Go SQLite (no CGo) |
| `github.com/google/uuid` | UUID generation |
| `golang.org/x/term` | Terminal detection and width |

**Note:** The plan originally specified `github.com/agnivade/levenshtein` for string similarity, but a hand-rolled Levenshtein implementation was used instead (`internal/analyze/suggest.go`) to keep external dependencies minimal. The similarity engine combines normalized Levenshtein distance with prefix/suffix bonuses and camelCase/underscore normalization.

## Architectural Decisions Made During Implementation

The following decisions were made during implementation and differ from or extend the original plan:

### Table Formatting (`internal/cli/table.go`)
A `Table` abstraction wraps `text/tabwriter` to provide consistent column-aligned output across all commands. It auto-detects TTY for bold headers and terminal width, falling back to 80 columns when piped. All output commands use this instead of raw `tabwriter`.

### Configuration System (`internal/config/`)
A dedicated `config` package manages `~/.dp/config.toml`. The root command's `PersistentPreRun` loads config and applies defaults for `--db` and `--json` flags when the user hasn't set them on the command line. Valid config keys: `db_path`, `default_source`, `known_tools`, `default_format`.

### JSON Output Mode
The `--json` global flag is defined on the root command and available to all subcommands via the `jsonOutput` package variable. When enabled, commands emit structured JSON to stdout instead of table output. The `default_format` config key can set JSON as the default.

### Build & Distribution
- `Makefile` with `build`, `test`, `vet`, `clean`, `install` targets
- `.goreleaser.yml` for cross-platform binary releases (linux/darwin/windows, amd64/arm64)
- CGO disabled in release builds (`CGO_ENABLED=0`)

### Public API (`pkg/desirepath/`)
The plan included a public Go library at `pkg/desirepath/` for programmatic integration. This has not yet been implemented; all logic currently lives in `internal/`.

## MVP Phases

See `docs/tasks/` for detailed task breakdowns:
- [Phase 1: Core](../tasks/phase-1-core.md) - Types, storage, record command (**complete**)
- [Phase 2: Reporting](../tasks/phase-2-reporting.md) - list, paths, stats, export commands (**complete**)
- [Phase 3: Suggestions & Aliases](../tasks/phase-3-suggestions.md) - suggest, alias, init commands (**complete**)
- [Phase 4: Polish](../tasks/phase-4-polish.md) - Output formatting, --json flag, config, tests (**complete**)
