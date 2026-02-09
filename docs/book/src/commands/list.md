# dp list

List recent desires

## Usage

    dp list [flags]

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| --since | "" | Duration or timestamp (30m, 24h, 7d, etc.) |
| --source | "" | Filter by source identifier |
| --tool | "" | Filter by tool name |
| --limit | 50 | Maximum number of desires to show |

## Examples

    $ dp list
    TIMESTAMP            SOURCE       TOOL           ERROR
    2026-02-09 14:32:15  claude-code  read_file      unknown tool
    2026-02-09 14:31:42  claude-code  file_read      tool not found
    2026-02-09 14:28:33  cursor       edit_document  invalid parameters
    2026-02-09 14:15:09  claude-code  grep_search    command failed
    2026-02-09 13:58:21  claude-code  write_file     permission denied

    5 desires shown (limit: 50)

    $ dp list --since 1h --source claude-code
    TIMESTAMP            SOURCE       TOOL        ERROR
    2026-02-09 14:32:15  claude-code  read_file   unknown tool
    2026-02-09 14:31:42  claude-code  file_read   tool not found
    2026-02-09 14:15:09  claude-code  grep_search command failed

    3 desires shown (limit: 50)

    $ dp list --tool read_file --limit 10
    TIMESTAMP            SOURCE       TOOL       ERROR
    2026-02-09 14:32:15  claude-code  read_file  unknown tool
    2026-02-08 16:22:44  cursor       read_file  file not found
    2026-02-08 11:05:33  claude-code  read_file  unknown tool

    3 desires shown (limit: 10)

## Details

The list command displays recent desire paths in reverse chronological order (newest first). Each row shows when the failure occurred, which AI tool generated it, the attempted tool name, and the error message.

Use `--since` to filter by time. Accepts durations like "30m", "2h", "7d", or absolute timestamps in RFC3339 format.

Use `--source` to filter by AI coding tool. This is useful when you're debugging integration issues with a specific tool.

Use `--tool` to filter by the attempted tool name. This helps identify recurring failures for a particular tool.

The `--limit` flag caps the number of results. Default is 50. Set to 0 for unlimited results (not recommended for large datasets).

Combine filters to narrow down results:

    $ dp list --since 24h --source claude-code --tool read_file

For programmatic access, use the global `--json` flag to get structured output.
