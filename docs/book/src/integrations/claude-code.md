# Claude Code

Ensure `dp` is available on the PATH inherited by the agent process, then run:

```bash
dp init --source claude-code
```

The default installer adds ingestion, signposting, and correction hooks;
[Using agents](README.md) lists them. Inspect `~/.claude/settings.json` after
installation. Existing settings are merged rather than replaced.

## Capture-only configuration

If you want only capture, merge the following into your existing settings and
retain any unrelated hooks. This is a manual alternative to the broader installer:

```json
{
  "hooks": {
    "PostToolUse": [{"matcher": ".*", "hooks": [{"type": "command", "command": "dp ingest --source claude-code"}]}],
    "PostToolUseFailure": [{"matcher": ".*", "hooks": [{"type": "command", "command": "dp ingest --source claude-code"}]}]
  }
}
```

Both events use the same ingest path. Errors become failure patterns; successful
calls remain invocations. `record` is a deprecated compatibility command.
`--track-all` is no longer an installation flag: ingestion captures both outcomes.

## Verify capture

First run the [isolated fixture](../getting-started.md#first-success-in-three-commands)
to test the binary and storage. Then generate a tool event in the agent and check
`dp list` and `dp paths`. A shell fixture proves ingestion, not that the agent
executed the configured hook.

If no events arrive, inspect PATH and the installed settings. Check that the
agent's user can write `~/.dp/desires.db`. The standard installed command hooks
are not marked asynchronous; no latency guarantee is implied here.

## Corrections and signposting

Aliases are stored mappings. [Pave](../commands/pave.md) describes the separate
PreToolUse interception setup. Signposting is optional search guidance with its
own [evaluation contract](https://github.com/scbrown/desire-path/blob/main/docs/plans/009-signposting-eval.md).

## Data handling

Captured inputs and errors can contain project data. Keep the local database
and [exports](../commands/export.md) within the intended audience.
See [configuration](../configuration.md) for database and capture settings.
