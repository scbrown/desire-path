# dp init

Install source-specific agent configuration:

```bash
dp init --source claude-code
```

The Claude Code installer merges `~/.claude/settings.json` and installs ingestion
for successful and failed tool calls, plus search-guidance and correction hooks.
See [Using agents](../integrations/README.md) for the complete hook list and
[capture-only configuration](../integrations/claude-code.md) for the minimal alternative.

## Flags

| flag | purpose |
|---|---|
| `--source` | Select a source plugin; see `dp sources` |
| `--settings` | Override the source-specific settings path |
| `--signpost-url` | Configure and verify a search backend |
| `--signpost-log` | Choose a signposting event log; `-` disables it |
| `--signpost-search-mode` | Request hybrid, semantic, or keyword search |
| `--skip-signpost-url-check` | Explicitly install before the backend is available |

Use `dp init --help` to inspect the installed version's flags. `--track-all` and
`--claude-code` are not flags in v0.2.1. Ingestion already captures both outcomes.
