# dp paths

Show aggregated paths ranked by frequency

## Usage

    dp paths [flags]

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| --top | 20 | Number of top paths to show |
| --since | "" | Filter by RFC3339 timestamp |

## Examples

    $ dp paths
    RANK  PATTERN        COUNT  FIRST_SEEN           LAST_SEEN            ALIAS
    1     read_file      142    2026-01-15 09:23:11  2026-02-09 14:32:15  Read
    2     file_read      89     2026-01-18 11:05:44  2026-02-09 14:31:42  Read
    3     grep_search    67     2026-01-20 08:15:22  2026-02-09 14:15:09  Grep
    4     edit_document  45     2026-01-22 13:44:33  2026-02-09 14:28:33  Edit
    5     write_file     38     2026-01-25 10:12:09  2026-02-09 13:58:21  Write
    6     search_grep    31     2026-01-28 15:33:44  2026-02-08 16:47:22  Grep
    7     bash_exec      28     2026-02-01 09:08:15  2026-02-07 18:22:33  Bash
    8     run_command    24     2026-02-03 12:55:09  2026-02-06 14:11:55  Bash
    9     file_write     19     2026-02-04 08:44:21  2026-02-05 16:33:12  Write
    10    glob_find      17     2026-02-05 11:22:44  2026-02-09 09:14:28  Glob

    Showing top 10 of 47 unique patterns

    $ dp paths --top 5 --since 2026-02-01T00:00:00Z
    RANK  PATTERN      COUNT  FIRST_SEEN           LAST_SEEN            ALIAS
    1     read_file    48     2026-02-01 08:15:33  2026-02-09 14:32:15  Read
    2     file_read    32     2026-02-01 09:22:11  2026-02-09 14:31:42  Read
    3     bash_exec    28     2026-02-01 09:08:15  2026-02-07 18:22:33  Bash
    4     run_command  24     2026-02-03 12:55:09  2026-02-06 14:11:55  Bash
    5     edit_file    21     2026-02-02 14:33:22  2026-02-08 11:44:09  Edit

    Showing top 5 of 29 unique patterns

## Details

The paths command aggregates desire records by tool name pattern and ranks them by frequency. This reveals which tool name variations are most commonly attempted by AI coding tools.

The ALIAS column shows if a mapping has been configured using `dp alias`. When an alias exists, desire_path can automatically find the correct tool name.

Use `--top` to control how many patterns to display. The default is 20, which typically covers the most actionable patterns.

Use `--since` to analyze patterns from a specific date forward. This is useful after making changes to your tool configuration to see if new patterns emerge.

This command is essential for:
- Identifying the most frequent tool name mismatches
- Prioritizing which aliases to create
- Understanding how different AI tools name the same capabilities
- Tracking whether integration improvements reduce failure rates

Pattern counts represent unique failure instances, not total attempts. Use `dp inspect` to drill into a specific pattern.
