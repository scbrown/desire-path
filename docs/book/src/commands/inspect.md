# dp inspect

Show detailed view of a specific desire path

## Usage

    dp inspect <pattern> [flags]

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| --since | "" | Duration or timestamp (30m, 24h, 7d, etc.) |
| --top | 5 | Number of top inputs/errors to show |

## Examples

    $ dp inspect read_file
    Pattern: read_file
    Total occurrences: 142
    Date range: 2026-01-15 09:23:11 to 2026-02-09 14:32:15
    Sources: claude-code (128), cursor (14)
    Alias: Read

    Activity by day:
    2026-02-09 ████████████████████ 24
    2026-02-08 ██████████████ 17
    2026-02-07 ████████ 9
    2026-02-06 ██████████ 12
    2026-02-05 ███████████ 13
    2026-02-04 ██████ 7
    2026-02-03 ████████████ 14
    2026-02-02 ███████████ 13
    2026-02-01 ████████ 10
    (earlier days: 23 total)

    Top errors:
    unknown tool                89 (62.7%)
    tool not found             31 (21.8%)
    invalid tool name          15 (10.6%)
    tool unavailable            7 (4.9%)

    Top inputs:
    {"path": "/etc/hosts"}                           18
    {"file_path": "/home/user/config.json"}          12
    {"path": "/var/log/app.log", "offset": 0}        9
    {"file_path": "/tmp/data.txt"}                   8
    {"path": "/home/user/.bashrc"}                   7

    $ dp inspect grep% --since 7d
    Pattern: grep% (SQL LIKE wildcard)
    Total occurrences: 45
    Date range: 2026-02-02 08:15:22 to 2026-02-09 14:15:09
    Sources: claude-code (41), cursor (4)
    Matched patterns: grep_search (31), grep_find (9), grep_files (5)

    Activity by day:
    2026-02-09 ██████████ 8
    2026-02-08 ████████ 6
    2026-02-07 ██████ 5
    2026-02-06 ████████ 7
    2026-02-05 ██████ 5
    2026-02-04 ████ 4
    2026-02-03 ██████ 5
    2026-02-02 ██████ 5

    Top errors:
    command failed             28 (62.2%)
    unknown tool              12 (26.7%)
    invalid parameters         5 (11.1%)

## Details

The inspect command provides a deep dive into a specific desire pattern. Use it to understand:

- How frequently the pattern occurs
- When it first appeared and when it was last seen
- Which AI tools are generating this pattern
- Whether an alias has been configured
- How activity trends over time
- What error messages are associated with it
- What input parameters are commonly attempted

The pattern argument supports SQL LIKE wildcards:
- Use `%` to match any sequence of characters: `grep%` matches `grep_search`, `grep_find`, etc.
- Use `_` to match any single character: `read_fil_` matches `read_file`, `read_fils`, etc.

Without wildcards, the pattern is matched exactly.

The histogram shows daily activity with a simple bar chart. The width of each bar represents relative frequency within the time window.

Use `--since` to focus on recent activity. This helps identify if a pattern is actively occurring or historical.

Use `--top` to control how many top errors and inputs are displayed. Default is 5, which usually captures the most common cases.

This command is invaluable for debugging specific integration issues and understanding why a particular tool name is failing.
