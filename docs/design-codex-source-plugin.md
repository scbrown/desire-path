# Design: OpenAI Codex CLI Source Plugin

**Issue:** dp-v0b
**Author:** polecat/slit
**Date:** 2026-02-09
**Status:** Draft

## Executive Summary

OpenAI Codex CLI lacks Claude Code-style per-tool-invocation hooks. The only
programmable hook (`notify`) fires on `agent-turn-complete`, not per-tool-use.
This means we cannot use the standard stdin-pipe model (`dp ingest --source X`)
that Claude Code and Kiro plugins use.

**Recommended approach:** A hybrid plugin with three integration paths:
1. **Notify hook** for real-time turn-level event capture
2. **Session transcript parser** for per-tool-invocation backfill from JSONL rollout files
3. **`codex exec --json` stream parser** for real-time per-tool capture in non-interactive mode

## Codex CLI Integration Points

| Mechanism | Granularity | Real-time? | Can block? | Notes |
|-----------|------------|------------|------------|-------|
| `notify` config | Per-turn | Yes | No | Only event: `agent-turn-complete` |
| `codex exec --json` | Per-item | Yes | No | NDJSON to stdout, exec mode only |
| Session rollout files | Per-item | No (post-hoc) | No | `~/.codex/sessions/<provider>/<date>/<uuid>.jsonl` |
| OpenTelemetry | Per-tool | Yes | No | Requires OTel collector setup |

**Critical gap:** No `pre_tool_use` or `post_tool_use` equivalent exists. OpenAI
rejected a community PR (#9796) for comprehensive hooks. Issue #2109 (388 thumbs-up)
remains open with no committed timeline.

## Codex Data Formats

### Notify Hook Payload (JSON argument)

```json
{
  "type": "agent-turn-complete",
  "thread-id": "uuid",
  "turn-id": "uuid",
  "cwd": "/path/to/dir",
  "input-messages": ["user prompt text"],
  "last-assistant-message": "assistant response text"
}
```

Note: `cwd` was added recently; no tool-level detail is included.

### `codex exec --json` NDJSON Events

```json
{"type":"thread.started","thread_id":"uuid"}
{"type":"turn.started"}
{"type":"item.started","item":{"id":"item_1","type":"command_execution","command":"bash -lc ls","status":"in_progress"}}
{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"bash -lc ls","status":"completed"}}
{"type":"item.started","item":{"id":"item_2","type":"file_change","status":"in_progress"}}
{"type":"item.completed","item":{"id":"item_2","type":"file_change","status":"completed"}}
{"type":"item.completed","item":{"id":"item_3","type":"agent_message","text":"Done."}}
{"type":"turn.completed","usage":{"input_tokens":24763,"cached_input_tokens":24448,"output_tokens":122}}
```

Item types: `command_execution`, `file_change`, `mcp_tool_call`, `agent_message`,
`reasoning`, `web_search`, `plan_update`

**Warning:** No formal JSON schema exists (Issue #1673 open). Field names have
changed between versions (e.g., `item_type` → `type`, `assistant_message` →
`agent_message` around v0.44.0). Our parser must be resilient to schema drift.

### Session Rollout Files (`~/.codex/sessions/<provider>/<date>/<uuid>.jsonl`)

- **Line 1:** `SessionMeta` — UUID, name, CWD, timestamps, model provider
- **Subsequent lines:** `ResponseItem` entries representing conversation turns
  including tool calls, tool results, and agent messages

The rollout format is richer than `codex exec --json` and includes the full
conversation history including tool inputs and outputs.

## Codex Config Locations

| Type | Path |
|------|------|
| User config | `~/.codex/config.toml` |
| Project config | `.codex/config.toml` |
| System config | `/etc/codex/config.toml` |
| Sessions | `~/.codex/sessions/<provider_id>/YYYY-MM-DD/<uuid>.jsonl` |
| History | `~/.codex/history.jsonl` |
| Auth | `~/.codex/auth_token.json` |
| Override dir | `$CODEX_HOME` (defaults to `~/.codex`) |

## Plugin Design

### Source Interface Implementation

```go
// internal/source/codex.go
type codex struct{}

func (c *codex) Name() string        { return "codex" }
func (c *codex) Description() string  { return "OpenAI Codex CLI" }
func (c *codex) Extract(raw []byte) (*Fields, error) { ... }
```

The `Extract` method must handle THREE distinct input formats:

#### Format 1: Notify hook payload (turn-level)

Triggered by `notify` config. The JSON arrives as a command-line argument to
the notify script, which pipes it to `dp ingest --source codex`.

```go
// Detected by: "type" == "agent-turn-complete"
// Maps to:
Fields{
    ToolName:   "agent-turn",           // synthetic tool name for turn events
    InstanceID: payload.ThreadID,
    CWD:        payload.CWD,
    Extra: {
        "turn_id":                payload.TurnID,
        "event_type":             "agent-turn-complete",
        "input_messages":         payload.InputMessages,
        "last_assistant_message": payload.LastAssistantMessage,
    },
}
```

#### Format 2: `codex exec --json` item events

Parsed from NDJSON stream by a wrapper script or `dp codex watch`.

```go
// Detected by: "type" == "item.completed" && "item" present
// Maps to:
Fields{
    ToolName:   item.Type,              // "command_execution", "file_change", etc.
    InstanceID: threadID,               // from earlier thread.started event
    CWD:        "",                     // not available per-item; set from session context
    ToolInput:  marshalItemFields(),    // command, status, text, etc.
    Error:      errorFromStatus(item),  // if status indicates failure
    Extra: {
        "item_id":    item.ID,
        "event_type": "item.completed",
        "turn_id":    currentTurnID,
    },
}
```

#### Format 3: Session rollout transcript line

Parsed by `dp codex import` or `dp ingest --source codex --transcript`.

```go
// Detected by: presence of session rollout fields
// Maps to: same as Format 2 but with richer data from transcript
```

### Installer Implementation

```go
// internal/source/codex.go (Installer interface)
func (c *codex) Install(opts InstallOpts) error { ... }
func (c *codex) IsInstalled(configDir string) (bool, error) { ... }
```

The installer modifies `~/.codex/config.toml` to add:

```toml
# Added by dp init --source codex
notify = ["dp", "ingest", "--source", "codex", "--notify"]
```

The `--notify` flag tells `dp ingest` that the JSON is passed as a CLI
argument (Codex notify style) rather than on stdin.

**Alternative:** Install a shell wrapper script that reads the JSON argument
and pipes it to stdin:

```bash
#!/bin/bash
# ~/.dp/codex-notify.sh
echo "$1" | dp ingest --source codex
```

```toml
notify = ["bash", "-c", "~/.dp/codex-notify.sh '$1'"]
```

### New CLI Commands

#### `dp codex watch` (new subcommand)

Wraps `codex exec` and parses the NDJSON stream in real-time:

```bash
# Usage:
dp codex watch -- codex exec "fix the bug"

# Internally:
# 1. Runs: codex exec --json "fix the bug"
# 2. Parses NDJSON from stdout
# 3. For each item.completed event, calls ingest pipeline
# 4. Passes non-JSON stderr through to terminal
```

This provides per-tool-invocation capture for non-interactive (exec) mode.

#### `dp codex import` (new subcommand)

Imports tool invocations from Codex session rollout files:

```bash
# Import latest session:
dp codex import --latest

# Import specific session:
dp codex import ~/.codex/sessions/chatgpt/2026-02-09/abc123.jsonl

# Import all sessions since a date:
dp codex import --since 2026-02-01
```

This provides historical backfill from existing Codex sessions.

## Mapping: Codex Item Types → desire_path Tool Names

| Codex Item Type | dp ToolName | Notes |
|----------------|-------------|-------|
| `command_execution` | `command_execution` | Shell command execution |
| `file_change` | `file_change` | File write/edit/create |
| `mcp_tool_call` | `mcp:{tool_name}` | Prefix with `mcp:` for disambiguation |
| `agent_message` | `agent_message` | Agent text response |
| `reasoning` | `reasoning` | Internal reasoning step |
| `web_search` | `web_search` | Web search invocation |
| `plan_update` | `plan_update` | Plan modification |
| `agent-turn-complete` | `agent_turn` | Turn-level event from notify hook |

## Schema Resilience Strategy

Since Codex has no stable JSON schema:

1. **Lenient parsing:** Use `json.RawMessage` for unknown fields
2. **Version detection:** Check for known field patterns to identify format version
3. **Graceful degradation:** Extract what we can, store rest in `Extra`
4. **Schema tests:** Test against known payloads from multiple Codex versions
5. **Forward-compatible:** New unknown fields stored in `Extra` automatically

## Implementation Plan

### Phase 1: Core Plugin (this PR)

1. **`internal/source/codex.go`** — Source plugin with `Extract()` for all three formats
2. **`internal/source/codex_test.go`** — Tests with representative payloads
3. **Installer** — Modifies `~/.codex/config.toml` for notify hook
4. **Register** in `init()` so `dp sources` shows it

### Phase 2: CLI Commands (follow-up)

5. **`dp codex watch`** — NDJSON stream parser wrapping `codex exec`
6. **`dp codex import`** — Session transcript importer

### Phase 3: Future (when Codex adds hooks)

7. **Per-tool hooks** — When/if Codex adds `pre_tool_use`/`post_tool_use`,
   update the installer and add a direct stdin-pipe path like Claude Code.
   OpenAI Issue #2109 tracking this.

## File Changes for Phase 1

| File | Action | Description |
|------|--------|-------------|
| `internal/source/codex.go` | Create | Source + Installer implementation |
| `internal/source/codex_test.go` | Create | Tests for Extract and Install |

No changes needed to:
- `internal/source/source.go` (existing interface sufficient)
- `internal/ingest/` (existing pipeline handles new sources)
- `internal/model/` (existing model handles new sources via Metadata)
- `internal/store/` (no schema changes needed)

## Open Questions

1. **Notify argument format:** Does Codex pass JSON as `$1` to the notify
   command, or does it pass the entire payload as a single stringified argument?
   Need to test with actual Codex installation.

2. **Rollout file format stability:** The session rollout JSONL format appears
   to be internal/unstable. Should we commit to parsing it, or wait for a
   stable API?

3. **`dp codex watch` vs skill:** Could we implement the NDJSON parser as a
   Codex Skill instead of a wrapper? A skill could use `dp ingest` directly.

4. **Config.toml editing:** The TOML installer needs to handle existing
   `notify` configuration gracefully (don't clobber existing notify hooks).
   Codex's `notify` config appears to be a single array, not composable like
   Claude Code's hook arrays.

## References

- [Codex CLI Docs](https://developers.openai.com/codex/cli/)
- [GitHub openai/codex](https://github.com/openai/codex)
- [Issue #2109 — Event hooks request (388 👍)](https://github.com/openai/codex/issues/2109)
- [Issue #1673 — JSON schema request](https://github.com/openai/codex/issues/1673)
- [PR #9796 — Comprehensive hooks (closed)](https://github.com/openai/codex/pull/9796)
- [Discussion #2150 — Hook feature discussion](https://github.com/openai/codex/discussions/2150)
