# Desire Path

Desire Path (`dp`) captures agent tool calls and turns repeated failures into
patterns you can query. It is for developers maintaining agent instructions,
tool names, and integrations. The CLI stores data in a local SQLite database;
source plugins interpret each agent's event format, and hooks connect capture
to the agent.

A *desire path* is a trail worn into grass where people choose to walk. Here,
a mistaken tool name is evidence of the interface an agent expected to find.
A repeated failure can suggest a better tool name, a missing feature, or an
instruction worth fixing.

## From failure to a fix

1. Ingest a call through a source plugin. Failed calls also become desires.
2. Query `dp paths` to find repeated names and `dp inspect` for the details.
3. Use `dp similar` to compare a mistaken name with known tools.
4. Record a mapping with `dp alias`, then improve instructions or configure
   [pave](commands/pave.md) if active interception is appropriate.

An alias alone stores a mapping; it does not automatically reroute every call.
The [first-success example](getting-started.md) demonstrates capture and query
without installing any hooks.

## Capabilities and boundaries

| capability | what it does |
|---|---|
| Patterns | Rank repeated failures and inspect their error messages |
| Suggestions | Compare names with known tools using string similarity |
| Aliases | Store intended tool mappings and command correction rules |
| Source plugins | Normalize calls from Claude Code, Codex, Cursor, and Kiro |
| Exports | Write failure or invocation records for offline analysis |
| Signposting | Offer search guidance through optional agent hooks |

Signposting observes weak searches and can offer a stack tool command. Its
evaluation contract, limitations, and measurements are in the
[signposting plan](https://github.com/scbrown/desire-path/blob/main/docs/plans/009-signposting-eval.md) and
[evaluation chapters](evaluations/README.md). It is separate from basic capture.

## Where to go next

- [Getting started](getting-started.md): install and produce a first result.
- [Using agents](integrations/README.md): inspect and configure hook capture.
- [Architecture](architecture.md): storage and plugin design.
- [The stack](stack.md): how the tools fit together.
- [Docs map](docs-map.md): design notes, historical plans, and supporting files.
