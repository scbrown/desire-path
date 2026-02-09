# dp stats

Show summary statistics

## Usage

    dp stats [flags]

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| --invocations | false | Show invocation stats instead of desires |

## Examples

    $ dp stats
    Desire Statistics

    Total desires: 1,247
    Unique tool patterns: 47
    Unique sources: 3
    Date range: 2026-01-15 09:23:11 to 2026-02-09 14:32:15

    Activity windows:
      Last 24 hours: 89 desires
      Last 7 days: 412 desires
      Last 30 days: 1,247 desires

    Top sources:
      claude-code: 1,089 (87.3%)
      cursor: 142 (11.4%)
      copilot: 16 (1.3%)

    Top desires:
      read_file: 142 (11.4%)
      file_read: 89 (7.1%)
      grep_search: 67 (5.4%)
      edit_document: 45 (3.6%)
      write_file: 38 (3.0%)

    Top tools (by alias):
      Read: 231 (18.5%)
      Grep: 98 (7.9%)
      Edit: 76 (6.1%)
      Write: 57 (4.6%)
      Bash: 52 (4.2%)

    $ dp stats --invocations
    Invocation Statistics

    Total invocations: 8,432
    Successful: 7,185 (85.2%)
    Failed: 1,247 (14.8%)
    Unique tools: 23
    Unique sources: 3
    Date range: 2026-01-15 09:23:11 to 2026-02-09 14:32:15

    Activity windows:
      Last 24 hours: 645 invocations (89 failures)
      Last 7 days: 3,128 invocations (412 failures)
      Last 30 days: 8,432 invocations (1,247 failures)

    Top sources:
      claude-code: 7,344 (87.1%)
      cursor: 1,028 (12.2%)
      copilot: 60 (0.7%)

    Top tools:
      Read: 2,847 (33.8%)
      Bash: 1,923 (22.8%)
      Edit: 1,204 (14.3%)
      Write: 891 (10.6%)
      Grep: 745 (8.8%)

    Failure rates by tool:
      Read: 231/2,847 (8.1%)
      Grep: 98/745 (13.2%)
      Edit: 76/1,204 (6.3%)
      Write: 57/891 (6.4%)
      Bash: 52/1,923 (2.7%)

## Details

The stats command provides a high-level overview of your desire_path data. It's useful for:

- Understanding the scale of tool call failures
- Identifying which AI tools have the most integration issues
- Tracking improvement over time
- Prioritizing which patterns to fix first

By default, stats shows desire (failure) data. Use `--invocations` to see statistics about all tool invocations, both successful and failed. Invocation tracking must be enabled with `dp init --track-all` for this data to be available.

Activity windows show rolling counts for the last 24 hours, 7 days, and 30 days. This helps identify trends: is the failure rate increasing, decreasing, or stable?

Top sources reveal which AI tools are generating the most failures. A high failure rate from one source might indicate a configuration issue or incompatibility.

Top desires show the most frequently attempted tool names. These are your highest-priority candidates for creating aliases.

When invocation tracking is enabled, the failure rate breakdown shows which tools have the highest error rates. This can reveal whether certain tools are more prone to naming mismatches or integration issues.

Run stats periodically to track the health of your AI coding tool integrations.
