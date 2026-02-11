# 008 - Turn Reconstruction via Transcript Parsing

## Problem

dp sees tool invocations in isolation. Each PostToolUse/PostToolUseFailure hook
fires once per tool call, so dp knows "Read was called" and "Bash was called"
but not that the agent needed 6 tool calls to accomplish what should have been 1.

**A "successful" tool call that requires 4 follow-ups to reach the actual goal
is a quiet failure.** The tool didn't error, but it didn't get the agent where
it needed to go. The number of tool calls in a turn is a proxy for tool
adequacy — long exploration turns mean the agent's intent didn't map cleanly
to available tools.

Examples of quiet failures:
- `Grep "TODO"` → `Read f1` → `Read f2` → `Read f3` — Grep worked but the
  agent had to manually chase context. A better tool would return relevant
  context directly.
- `Bash "ls src/"` → `Bash "ls src/utils/"` → `Read helpers.go` — navigating
  blind. `Glob` with the right pattern is one call.
- `Glob "**/*.test.ts"` → `Read t1` → `Read t2` → `Grep "describe"` →
  `Read t3` — hunting for a specific test. No single tool does "find the
  test for X."

**Turn length and shape become desire paths.** Not because tools failed, but
because the journey was longer than it should have been. These patterns tell
us which tools need clarification, which need new features, and where new
tools should be created.

## Transcript Format Reference

Claude Code transcripts are JSONL files at:
```
~/.claude/projects/<project-slug>/<session-uuid>.jsonl
```

Subagent transcripts at:
```
~/.claude/projects/<project-slug>/<session-uuid>/subagents/agent-<id>.jsonl
```

### Key Schema Facts (from dp-k74 research)

1. **Each content block is a separate JSONL event.** An API response with
   `[thinking, text, tool_use, tool_use]` = 4 separate assistant events
   chained via `parentUuid`.

2. **`parentUuid` forms a tree**, not a flat list. Events chain: each event
   points to its parent.

3. **Tool results are separate user events**, each containing one
   `tool_result` block. `sourceToolAssistantUUID` links result → request.

4. **Turn boundaries** are marked by system events:
   - `subtype: "stop_hook_summary"` — end of assistant activity
   - `subtype: "turn_duration"` — total wall time with `durationMs`
   - `subtype: "compact_boundary"` — conversation was compressed

5. **Event types**: `user` (human text or tool results), `assistant`
   (text/thinking/tool_use — one per event), `progress` (hooks, bash
   streaming, subagent activity), `system` (turn boundaries, compaction),
   `file-history-snapshot`.

6. **`toolUseResult`** on user events contains tool-specific output shapes
   (Bash: stdout/stderr, Read: file content, Edit: patch, Task: agent
   metrics, etc.)

## Decisions

- **Long turn threshold**: Configurable via `turn_length_threshold` in
  `config.toml`, defaulting to 5. Surfaced as `--min-length` on `dp turns`.
- **Subagent calls**: Do NOT count toward parent turn length. Subagent
  transcripts are parsed independently — they're their own turns with their
  own patterns. The parent turn sees only the `Task` tool call itself.
- **Pattern fuzzing**: Consecutive repeats collapse into `Tool{N+}` for
  clustering (so `Read{3}` and `Read{5}` match the same abstract pattern),
  but `dp turns` can drill down to show exact sequences per instance.
- **No backfill**: This is a new feature. No retroactive enrichment or
  `dp enrich` command needed. Turn data flows in from transcript parsing
  going forward.
- **No backwards compatibility**: New schema columns can be NOT NULL with
  defaults. No migration path for old data.

## Design

### Phase 1: Transcript Parser

New package: `internal/transcript/`

```go
package transcript

// Parse reads Claude Code transcript JSONL and returns structured turns.
func Parse(r io.Reader) ([]Turn, error)

// Turn represents one human→model→human cycle.
type Turn struct {
    SessionID  string
    Index      int           // 0-based turn number in session
    StartedAt  time.Time
    DurationMs int           // from turn_duration system event, 0 if absent
    Steps      []Step        // tool calls in execution order
}

// Step represents one tool invocation within a turn.
type Step struct {
    ToolName   string
    ToolUseID  string
    Input      json.RawMessage // tool input parameters
    Sequence   int             // 0-based position in turn
    IsParallel bool            // true if fired concurrently with adjacent steps
    IsError    bool
    Error      string
}
```

**Parsing algorithm:**

1. Read all JSONL events, build `uuid → event` index.
2. Walk events in timestamp order.
3. Identify turn starts: `user` events with string content (human text).
4. Identify turn ends: `system` events with `subtype: "stop_hook_summary"`.
5. Between start and end, collect all `assistant` events with `tool_use`
   content blocks — these are the Steps.
6. Match each tool_use to its result via `sourceToolAssistantUUID` on the
   corresponding user/tool_result event.
7. Determine sequence by timestamp order within the turn.
8. Detect parallelism: consecutive tool_use events that chain from the same
   parent (no intervening tool_result) are parallel.

**Subagent handling:** Subagent transcripts are parsed as independent
sessions. They produce their own Turn/Step data. The parent session's `Task`
tool call is just a single Step — the subagent's internal work is analyzed
separately.

### Phase 2: Schema Migration + Ingest Integration

Add V3 migration to `internal/store/sqlite.go`:

```sql
ALTER TABLE invocations ADD COLUMN turn_id TEXT NOT NULL DEFAULT '';
ALTER TABLE invocations ADD COLUMN turn_sequence INTEGER NOT NULL DEFAULT 0;
ALTER TABLE invocations ADD COLUMN turn_length INTEGER NOT NULL DEFAULT 0;
```

| Column | Type | Purpose |
|--------|------|---------|
| `turn_id` | TEXT | Groups invocations within a turn. Derived from `(session_id, turn_index)`. |
| `turn_sequence` | INTEGER | 0-based position of this call within the turn. |
| `turn_length` | INTEGER | Total tool calls in this turn (denormalized). High value = long exploration turn. |

**Why `turn_length` is denormalized:** The primary query is "show me tools
that appear in long turns." Without `turn_length` on each row, every query
needs a subquery to count siblings. With it, `WHERE turn_length > N` is
trivial.

Model change in `internal/model/model.go`:

```go
type Invocation struct {
    // ... existing fields ...
    TurnID       string `json:"turn_id,omitempty"`
    TurnSequence int    `json:"turn_sequence"`
    TurnLength   int    `json:"turn_length"`
}
```

**Ingest integration:** Modify the ingest pipeline so that when a transcript
path is available (it's already in `Fields.Extra` from the Claude Code
hook), dp can parse the transcript to determine turn context for the current
invocation. The hook fires per-tool-call, so at ingest time we know the
`session_id` and `tool_use_id` — we just need to locate the current turn in
the transcript to populate `turn_id`, `turn_sequence`, and `turn_length`.

**Transcript location:** The hook payload includes `transcript_path`. For
the in-progress session, this file is being actively written. The parser
reads up to the current point, identifies which turn contains the
`tool_use_id` being ingested, and extracts the turn metadata. Since tool
results arrive after tool calls, by the time PostToolUse fires, the
tool_use event and all preceding events in the turn are already written.

### Phase 3: Turn-Aware Reporting

#### `dp paths --turns`

Extend the paths view with turn context:

```
RANK  PATTERN     COUNT  AVG_TURN_LEN  LONG_TURN_%  ALIAS
1     Grep        89     4.2           35%          -
2     Read        203    3.1           18%          -
3     Bash        67     5.8           52%          -
```

- **AVG_TURN_LEN**: Average turn length when this tool appears.
- **LONG_TURN_%**: Percentage of appearances in turns exceeding the
  configured threshold (default 5).

High `LONG_TURN_%` signals: "when agents use this tool, they usually need
many more calls to finish — the tool isn't getting them there."

#### `dp turns`

New command showing turn-level data:

```
dp turns [--min-length N] [--since DATETIME] [--session SESSION_ID] [--json]
```

Default output (turns exceeding threshold):

```
SESSION   TURN  LENGTH  TOOLS
abc123    3     7       Grep → Read → Read → Read → Edit → Read → Edit
abc123    5     5       Bash → Bash → Read → Bash → Read
def456    1     8       Glob → Read → Read → Grep → Read → Read → Read → Edit
```

#### `dp turns --patterns`

Cluster similar turn shapes using abstract patterns where consecutive
repeats of the same tool collapse into `Tool{N+}`:

```
PATTERN                          COUNT  AVG_LENGTH  SESSIONS
Grep → Read{2+} → Edit          12     5.3         4
Glob → Read{3+}                 8      4.8         3
Bash{2+} → Read                 6      3.5         5
```

Drill down into a specific pattern to see exact instances:

```
dp turns --pattern "Grep → Read{2+} → Edit"
```

Shows every matching turn with its exact tool sequence.

### Phase 4: Desire Path Surfacing

Connect turn patterns back to the desire path system. Long, recurring turn
patterns are desire paths even though no tool errored.

**New desire category:** `"turn-pattern"` (alongside existing `"env-need"`).

When `dp turns --patterns` (or an analysis pass during ingest) detects a
turn pattern that:
- Appears 3+ times across sessions
- Has turn length exceeding the configured threshold

It creates a Desire record:
- `ToolName`: the first tool in the pattern (the "entry point")
- `Error`: descriptive, e.g., "Repeated pattern: Grep → Read{2+} → Edit
  (avg 5.3 calls, seen 12 times across 4 sessions)"
- `Category`: `"turn-pattern"`
- `Source`: `"transcript-analysis"`

These show up in `dp paths` alongside regular failures, giving a unified
view of "what agents struggle with" — both hard failures and soft ones.

## Privacy

The parser extracts only structural metadata: tool names, sequence, timing,
error messages. It never stores user prompts, tool input content, or tool
output in dp's database.

## Config

New `config.toml` key:

```toml
turn_length_threshold = 5  # turns with more tool calls than this are "long"
```

Used by:
- `dp turns` default `--min-length` value
- `dp paths --turns` `LONG_TURN_%` calculation
- Desire path surfacing threshold

## Files Modified

| File | Changes |
|------|---------|
| `internal/transcript/parse.go` | **New**: JSONL parser, turn extraction |
| `internal/transcript/parse_test.go` | **New**: Unit tests with fixture transcripts |
| `internal/transcript/testdata/` | **New**: Minimal synthetic JSONL fixtures |
| `internal/model/model.go` | Add TurnID, TurnSequence, TurnLength to Invocation |
| `internal/store/sqlite.go` | V3 migration, updated insert/query for turn columns |
| `internal/store/store.go` | Add TurnStats query methods |
| `internal/config/config.go` | Add TurnLengthThreshold field |
| `internal/ingest/ingest.go` | Transcript parsing at ingest time, turn-pattern desires |
| `internal/cli/turns.go` | **New**: `dp turns` and `dp turns --patterns` |
| `internal/cli/paths.go` | Add `--turns` flag, AVG_TURN_LEN/LONG_TURN_% columns |
