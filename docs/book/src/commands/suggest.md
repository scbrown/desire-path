# dp suggest

Suggest known tool mappings for a tool name

## Usage

    dp suggest <tool-name> [flags]

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| --known | "" | Comma-separated list of known tools (overrides defaults) |
| --threshold | 0.5 | Minimum similarity score (0.0 to 1.0) |
| --top | 5 | Maximum number of suggestions to show |

## Examples

    $ dp suggest read_file
    Checking alias mappings...
    Found alias: read_file -> Read

    Suggestions for "read_file":
    1. Read     (score: 0.82, reason: alias)

    Recommended action: Use "Read" instead of "read_file"

    $ dp suggest file_reader
    Checking alias mappings...
    No alias found for "file_reader"

    Suggestions for "file_reader":
    1. Read     (score: 0.73)
    2. Write    (score: 0.58)
    3. Edit     (score: 0.56)

    Recommended action: Consider using "Read" or create an alias with:
        dp alias file_reader Read

    $ dp suggest grepsearch --threshold 0.4
    Checking alias mappings...
    No alias found for "grepsearch"

    Suggestions for "grepsearch":
    1. Grep        (score: 0.78)
    2. WebSearch   (score: 0.45)
    3. Glob        (score: 0.42)

    Recommended action: Consider using "Grep" or create an alias with:
        dp alias grepsearch Grep

    $ dp suggest custom_tool --known "CustomRead,CustomWrite,CustomEdit"
    Checking alias mappings...
    No alias found for "custom_tool"

    Suggestions for "custom_tool":
    1. CustomEdit   (score: 0.54)
    2. CustomWrite  (score: 0.51)

    No strong matches found. Consider checking the tool name.

## Details

The suggest command helps resolve tool name mismatches by finding the closest matching known tool. It uses two strategies:

1. Alias lookup: First checks if an explicit alias has been configured with `dp alias`
2. Similarity matching: Calculates Levenshtein distance to find phonetically or structurally similar tool names

Default known tools (Claude Code conventions):
- Read
- Write
- Edit
- Bash
- Glob
- Grep
- Task
- WebFetch
- WebSearch
- NotebookEdit

Override the known tools list with `--known` for custom tool environments:

    dp suggest mytool --known "Tool1,Tool2,Tool3"

The similarity score ranges from 0.0 (completely different) to 1.0 (identical). The `--threshold` flag filters out weak matches. Default is 0.5, which typically excludes spurious suggestions.

Use `--top` to limit suggestions. Default is 5, which is usually sufficient to find the right match.

When an alias exists, it's always shown first with a score of 1.0 and marked as "alias". This makes aliases the authoritative source for mappings.

Use suggest interactively when debugging why a tool call failed:

    $ dp list --limit 1
    # See a failed tool name
    $ dp suggest <that-tool-name>
    # Get suggestions and create alias if needed

Integrate suggest into your workflow by running it after reviewing `dp paths` to batch-create aliases for the most common patterns.
