# Phase 2: Reporting

Read-only commands for viewing and analyzing collected desires.

## Tasks

### 2.1 List command (`internal/cli/list.go`)
- [ ] `dp list` - Show recent desires in a table
- [ ] `--since` flag - Duration filter (e.g., `7d`, `24h`, `30m`)
- [ ] `--source` flag - Filter by source (e.g., `claude-code`)
- [ ] `--tool` flag - Filter by tool_name
- [ ] `--limit` flag - Max results (default 50)
- [ ] Output columns: timestamp, source, tool_name, error (truncated)

### 2.2 Paths command (`internal/cli/paths.go`)
- [ ] `dp paths` - Show aggregated paths ranked by frequency
- [ ] `--top` flag - Limit results (default 20)
- [ ] `--since` flag - Duration filter
- [ ] Output columns: rank, pattern, count, first_seen, last_seen, alias
- [ ] SQL aggregation: `GROUP BY tool_name ORDER BY COUNT(*) DESC`

### 2.3 Stats command (`internal/cli/stats.go`)
- [ ] `dp stats` - Summary statistics
- [ ] Total desires recorded
- [ ] Unique tool names (paths)
- [ ] Top 5 sources
- [ ] Top 5 most common desires
- [ ] Date range (earliest to latest)
- [ ] Desires in last 24h / 7d / 30d

### 2.4 Export command (`internal/cli/export.go`)
- [ ] `dp export` - Dump raw data
- [ ] `--format json` (default) - JSON array to stdout
- [ ] `--format csv` - CSV to stdout
- [ ] `--since` flag for filtering
- [ ] Suitable for piping to `jq`, spreadsheets, etc.

### 2.5 Inspect command (`internal/cli/inspect.go`)
- [ ] `dp inspect <pattern>` - Detailed view of a specific path
- [ ] Show all desires matching the pattern
- [ ] Show frequency over time (simple text histogram)
- [ ] Show most common tool_input values
- [ ] Show most common error messages

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
