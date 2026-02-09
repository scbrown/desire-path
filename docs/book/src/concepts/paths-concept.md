# Paths

A **path** is an aggregated pattern of repeated desires. When the same tool name fails multiple times, it forms a well-worn trail — a signal that this capability is genuinely needed.

## The Metaphor

Think of a university campus. Planners lay out sidewalks where they think people should walk. But students take shortcuts across the grass. Over time, these shortcuts become visible worn paths. Smart planners pave over the desire paths because they reveal where sidewalks actually belong.

Desire paths in AI tooling work the same way. When Claude Code repeatedly tries to call `read_file` and fails, that repeated pattern tells you: "Build this. I need this."

## What Paths Show

Each path aggregates desires by `tool_name` and shows:

- **pattern**: The tool name that keeps failing (e.g., `read_file`)
- **count**: How many times it's failed
- **first_seen**: When this pattern first appeared
- **last_seen**: Most recent occurrence
- **alias_to**: If you've mapped this to a real tool (optional)

## Viewing Paths

Use `dp paths` to see the patterns:

```bash
dp paths
```

Output:

```
PATTERN         COUNT   FIRST SEEN            LAST SEEN             ALIAS
read_file       47      2026-02-01 09:15:23   2026-02-09 10:30:45   Read
execute_bash    23      2026-02-03 14:22:10   2026-02-09 08:12:33
list_dir        15      2026-02-05 11:05:42   2026-02-08 16:44:21   Glob
write_output    8       2026-02-07 13:30:12   2026-02-09 09:18:55
```

## What To Do With Paths

### High Count = High Priority

A tool that fails 47 times is screaming "I should exist." That's your top priority for building or aliasing.

### Low Count = Wait and Watch

A tool that fails twice might be a one-off mistake or an edge case. Don't build infrastructure for noise.

### Recent Activity = Active Pain Point

If `last_seen` is today and `count` is climbing, this is actively blocking work right now.

## Paths vs. Desires

- **Desires** are raw events: individual failures with full context
- **Paths** are aggregated insights: patterns showing what matters

You don't fix individual desires. You fix paths. The frequency tells you what to prioritize.

## Example Workflow

1. Check paths: `dp paths`
2. See `read_file` has failed 47 times
3. Discover your tool is actually called `Read`
4. Create an alias: `dp alias read_file Read`
5. Now `dp paths` shows the mapping

Paths reveal the problem. Aliases (or building new tools) solve it.
