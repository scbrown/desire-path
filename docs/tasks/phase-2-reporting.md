# Phase 2: Reporting

Read-only commands for viewing and analyzing collected desires.

## Tasks

### 2.1 List command (`internal/cli/list.go`)
- [x] `dp list` - Show recent desires in a table
- [x] `--since` flag - Duration filter (e.g., `7d`, `24h`, `30m`)
- [x] `--source` flag - Filter by source (e.g., `claude-code`)
- [x] `--tool` flag - Filter by tool_name
- [x] `--limit` flag - Max results (default 50)
- [x] Output columns: timestamp, source, tool_name, error (truncated)

### 2.2 Paths command (`internal/cli/paths.go`)
- [x] `dp paths` - Show aggregated paths ranked by frequency
- [x] `--top` flag - Limit results (default 20)
- [x] `--since` flag - Duration filter
- [x] Output columns: rank, pattern, count, first_seen, last_seen, alias
- [x] SQL aggregation: `GROUP BY tool_name ORDER BY COUNT(*) DESC`

### 2.3 Stats command (`internal/cli/stats.go`)
- [x] `dp stats` - Summary statistics
- [x] Total desires recorded
- [x] Unique tool names (paths)
- [x] Top 5 sources
- [x] Top 5 most common desires
- [x] Date range (earliest to latest)
- [x] Desires in last 24h / 7d / 30d

### 2.4 Export command (`internal/cli/export.go`)
- [x] `dp export` - Dump raw data
- [x] `--format json` (default) - JSON array to stdout
- [x] `--format csv` - CSV to stdout
- [x] `--since` flag for filtering
- [x] Suitable for piping to `jq`, spreadsheets, etc.

### 2.5 Inspect command (`internal/cli/inspect.go`)
- [x] `dp inspect <pattern>` - Detailed view of a specific path
- [x] Show all desires matching the pattern
- [x] Show frequency over time (simple text histogram)
- [x] Show most common tool_input values
- [x] Show most common error messages

## Done when

```bash
# After recording several desires:
./dp list --since 7d
./dp paths --top 10
./dp stats
./dp export --format json | jq '.[0]'
./dp inspect read_file
# All produce meaningful output
```

## Depends on

- Phase 1 (Core)

## Blocks

- Phase 4 (Polish - table formatting improvements)
