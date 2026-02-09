# Getting Started

This guide walks through installing dp, connecting it to Claude Code, and running your first analysis.

## Installation

### Option 1: Install via `go install` (Recommended)

If you have Go 1.24+ installed:

```bash
go install github.com/scbrown/desire-path/cmd/dp@latest
```

Make sure `$GOPATH/bin` (or `$HOME/go/bin`) is in your `PATH`.

### Option 2: Install from Source

Clone the repository and build:

```bash
git clone https://github.com/scbrown/desire-path.git
cd desire-path
make install
```

This builds the `dp` binary and copies it to `$GOPATH/bin`.

### Option 3: Download a Binary Release

Visit the [GitHub Releases page](https://github.com/scbrown/desire-path/releases) and download the pre-built binary for your platform. Extract it and move the `dp` binary somewhere in your `PATH`.

## Set Up Claude Code Integration

dp works by hooking into Claude Code's event system. Run:

```bash
dp init --source claude-code
```

This command updates `~/.claude/settings.json` to add a `PostToolUseFailure` hook that runs `dp record --source claude-code` whenever a tool call fails. The operation is idempotent—safe to run multiple times without duplicating hooks.

### What Just Happened?

`dp init` added a JSON snippet to your Claude Code settings:

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

Now every time Claude Code attempts a tool call that fails, the hook fires asynchronously, passing the failure payload to `dp record`. The command parses the JSON, extracts universal fields (tool name, session ID, error message, working directory), and writes a desire record to `~/.dp/desires.db`.

Claude Code continues immediately—dp runs in the background and won't slow down your session.

## Accumulate Desires

Just use Claude Code normally. Every tool call failure is now being recorded. After a few sessions, you'll have data to analyze.

If you want to test the system right away without waiting for real failures, you can manually inject a fake desire:

```bash
echo '{"tool_name":"read_file","error":"unknown tool","session_id":"test","cwd":"/tmp"}' | dp record --source claude-code
```

Check that it was recorded:

```bash
dp list
```

You should see your test desire (or real ones, if you've been using Claude Code since running `dp init`).

## First Analysis

### List Desires

Show the raw failures:

```bash
dp list
```

Add filters:

```bash
# Only desires from the last 24 hours
dp list --since 24h

# Only desires matching a specific tool name
dp list --tool read_file

# Limit to 10 results
dp list --limit 10
```

### View Aggregated Paths

Paths are aggregated desire patterns ranked by frequency:

```bash
dp paths
```

This shows which tool names failed most often, how many times each failed, and when they were first and last seen.

### Inspect a Specific Pattern

Dive deep into a single tool name:

```bash
dp inspect read_file
```

This returns:
- Total occurrences
- First/last seen timestamps
- Histogram of failures over time
- Top error messages
- Top input payloads (truncated)
- Whether an alias already exists

### Suggest Close Matches

Find known tools similar to a hallucinated name:

```bash
dp suggest read_file
```

dp uses Levenshtein distance with camelCase normalization, prefix bonuses, and suffix bonuses to rank known tools by similarity. By default it shows the top 5 matches with scores above 0.5.

The known tools list is configurable—see [Configuration](./configuration.md) for details.

### Create an Alias

Once you've identified the correct real tool, wire up the alias:

```bash
dp alias read_file Read
```

Now any system consuming the dp database can map `read_file` → `Read`. For example, a Claude Code plugin could intercept tool calls, check the aliases table, and rewrite the tool name before execution.

dp doesn't currently perform this rewriting automatically—it just stores the mapping. You can list all aliases with:

```bash
dp aliases
```

Delete an alias:

```bash
dp alias --delete read_file
```

## Optional: Track All Invocations

By default, dp only captures failures (via `PostToolUseFailure`). If you want to track *all* tool calls—successes and failures—for deeper analysis (like success rates, invocation frequency, session analysis), enable full tracking:

```bash
dp init --source claude-code --track-all
```

This adds two additional hooks:
- `PostToolUse → dp ingest --source claude-code`
- `PostToolUseFailure → dp ingest --source claude-code`

The `ingest` command writes invocation records (not desire records). Invocations include a boolean `is_error` field to distinguish successes from failures. This generates significantly more data—every tool call fires the hook—so only enable it if you need invocation-level analytics.

View invocation stats:

```bash
dp stats --invocations
```

Export invocation data:

```bash
# Export as JSON
dp export --type invocations

# Export as CSV
dp export --type invocations --format csv > invocations.csv

# Filter by date
dp export --type invocations --since 2026-02-01
```

## Next Steps

- Customize configuration: [Configuration Reference](./configuration.md)
- Learn about the data model: [Concepts](./concepts/README.md)
- Explore all commands: [Command Reference](./commands/README.md)
- Integrate other AI tools: [Integrations](./integrations/README.md)
- Understand the internals: [Architecture](./architecture.md)
