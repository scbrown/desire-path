# Using agents

A source plugin translates an agent event into an invocation: tool name, inputs,
error, session, and working directory. Failed invocations also become desires.
The release's `dp sources` command lists the plugins and installer status.

## Claude Code

```bash
dp init --source claude-code
dp sources
```

The installer merges `~/.claude/settings.json`. In v0.2.1 it adds:

| event | command | purpose |
|---|---|---|
| PostToolUse and PostToolUseFailure | `dp ingest --source claude-code` | Capture calls and failures |
| PreToolUse, Bash | `dp signpost-prefetch` | Prepare optional search guidance |
| PostToolUse and PostToolUseFailure, Bash | `dp signpost` | Offer search guidance |
| PostToolUseFailure | `dp pave-correct` | Consult correction rules |

Review the settings after installation. These entries use command hooks without
an `async` flag; do not assume capture has zero latency. For a capture-only
configuration and verification, see [Claude Code](claude-code.md).
Signposting needs a search backend; see the [evaluation plan](https://github.com/scbrown/desire-path/blob/main/docs/plans/009-signposting-eval.md)
and `dp init --help` for the backend flags. [Pave](../commands/pave.md) covers
active interception as a separate opt-in.

## Other source plugins

`dp sources` in v0.2.1 lists `codex`, `cursor`, and `kiro` as well as `claude-code`.
Use the installed binary's `dp init --help` and inspect generated configuration
for the chosen source. A parser or installer being present does not establish
that every event is emitted by every agent version. The
[Codex design note](https://github.com/scbrown/desire-path/blob/main/docs/design-codex-source-plugin.md) records its protocol.

For custom event producers, pipe their supported payloads into
`dp ingest --source <name>`. See [writing a source plugin](writing-plugins.md)
for the interface and [architecture](../architecture.md) for data flow.
