# 008 - Turn Reconstruction via Transcript Parsing

## Context

dp currently ingests tool invocations one at a time via Claude Code hooks
(PostToolUse / PostToolUseFailure). Each hook fires once per tool call, so dp
sees individual invocations in isolation. It knows "Read was called" and "Bash
was called" but not "Read and Bash were called in parallel as part of the same
assistant turn" or "this Bash call was the 3rd tool call in a 7-step reasoning
chain."

Turn-level context unlocks:
- **Pattern detection**: "Agent always calls Glob then Read then Edit" — a
  3-step desire path that could be a single command.
- **Parallel call analysis**: Which tools does the agent fire in parallel?
  Do parallel calls to the same tool indicate confusion?
- **Error context**: A failed tool call after 5 successful ones in the same
  turn is different from a failed first call.
- **Session replay**: Reconstruct what the agent actually did, not just which
  tools it touched.

## Transcript Schema (Claude Code 2.1.x)

Transcripts are JSONL files at:
```
~/.claude/projects/<project-slug>/<session-uuid>.jsonl
```

Subagent transcripts at:
```
~/.claude/projects/<project-slug>/<session-uuid>/subagents/agent-<id>.jsonl
```

### Event Types

| Type | Purpose | Key Fields |
|------|---------|------------|
| `file-history-snapshot` | File backup tracking | `messageId`, `snapshot`, `isSnapshotUpdate` |
| `user` | Human input or tool results | `message`, `parentUuid`, `uuid`, `sessionId` |
| `assistant` | Model output (streamed per content block) | `message`, `parentUuid`, `uuid`, `requestId` |
| `progress` | Hook execution, bash output, agent progress | `data.type`, `toolUseID`, `parentToolUseID` |
| `system` | Turn boundaries, compaction markers | `subtype`, `durationMs`, `hookCount` |

### Common Fields (all event types)

```
parentUuid      string    Parent event UUID (forms a tree, not a flat list)
uuid            string    Unique event ID
sessionId       string    Session UUID (matches filename)
type            string    Event type discriminator
timestamp       string    RFC3339 timestamp
cwd             string    Working directory at time of event
gitBranch       string    Active git branch
version         string    Claude Code version (e.g., "2.1.39")
isSidechain     bool      Whether event is part of a sidechain
userType        string    Always "external" for main session
slug            string    Session slug (human-readable name, may be absent)
```

### Critical Finding: Streaming Content Blocks

**Each assistant content block is a separate JSONL event.** An API response
with `[thinking, text, tool_use, tool_use]` appears as 4 separate assistant
events chained via `parentUuid`. This means:

- A single assistant message with 3 parallel tool calls = 3 separate events
- The events chain: `text_event.uuid → tool1.parentUuid`, `tool1.uuid → tool2.parentUuid`
- There is NO multi-tool-use event — each tool_use gets its own event

### User Messages

Two shapes:

**Human input** (text message from user):
```json
{
  "type": "user",
  "message": {"role": "user", "content": "fix the bug"},
  "parentUuid": "<previous-event>",
  "uuid": "<this-event>",
  "thinkingMetadata": {"maxThinkingTokens": 31999},
  "permissionMode": "bypassPermissions",
  "todos": []
}
```

**Tool results** (one result per event):
```json
{
  "type": "user",
  "message": {
    "role": "user",
    "content": [{"tool_use_id": "toolu_xxx", "type": "tool_result", "content": "...", "is_error": false}]
  },
  "sourceToolAssistantUUID": "<assistant-event-that-requested-tool>",
  "toolUseResult": { /* tool-specific result structure */ },
  "parentUuid": "<assistant-tool_use-event>"
}
```

Key: `sourceToolAssistantUUID` links the result back to the assistant event
that contained the `tool_use` block. This is the join key for pairing
requests with responses.

### Assistant Messages

Always contain exactly ONE content block per event:

**Text block:**
```json
{"message": {"content": [{"type": "text", "text": "..."}], "stop_reason": "..."}}
```

**Thinking block:**
```json
{"message": {"content": [{"type": "thinking", "thinking": "...", "signature": "..."}]}}
```

**Tool use block:**
```json
{"message": {"content": [{"type": "tool_use", "id": "toolu_xxx", "name": "Bash", "input": {...}}]}}
```

The `message` field may be a JSON string (needs double-parse) or an object.

### toolUseResult Shapes

The `toolUseResult` field on user/tool_result events varies by tool:

| Tool | Keys |
|------|------|
| Bash | `stdout`, `stderr`, `interrupted`, `isImage`, `noOutputExpected` |
| Edit | `filePath`, `oldString`, `newString`, `originalFile`, `structuredPatch`, `replaceAll`, `userModified` |
| Read | `file`, `type` |
| Write | `filePath`, `content`, `originalFile`, `structuredPatch`, `type` |
| Grep | `content`/`filenames`, `mode`, `numFiles`, `numLines` |
| Glob | `filenames`, `durationMs`, `numFiles`, `truncated` |
| Task (agent) | `agentId`, `content`, `prompt`, `status`, `totalDurationMs`, `totalTokens`, `totalToolUseCount`, `usage` |
| TaskCreate/Update | `task`, `success`, `taskId`, `updatedFields`, `statusChange` |
| EnterPlanMode | `message` |
| AskUserQuestion | `questions`, `answers` |

### System Events

| Subtype | Purpose | Key Fields |
|---------|---------|------------|
| `stop_hook_summary` | End of assistant turn, hooks ran | `hookCount`, `hookInfos`, `hookErrors`, `preventedContinuation` |
| `turn_duration` | Total turn wall time | `durationMs` |
| `compact_boundary` | Context was compressed | `compactMetadata.trigger`, `compactMetadata.preTokens` |

### Progress Events

| data.type | Purpose |
|-----------|---------|
| `hook_progress` | Hook executing (PreToolUse, PostToolUse, Stop, SessionStart) |
| `bash_progress` | Streaming bash output |
| `agent_progress` | Subagent activity |
| `waiting_for_task` | Waiting on background task |

### Turn Boundaries

A "turn" (human → model → human) can be identified by:

1. **Start**: A `user` event where `message.content` is a string (human text),
   not a list of tool_results.
2. **End**: A `system` event with `subtype: "stop_hook_summary"` followed by
   `subtype: "turn_duration"`.
3. **Everything between**: assistant events, tool results, progress events,
   all linked via `parentUuid` tree.

## Proposed Data Model

### Option A: Turn + Sequence columns on invocations (Recommended)

Add columns to the existing invocations table:

```sql
ALTER TABLE invocations ADD COLUMN turn_id TEXT;
ALTER TABLE invocations ADD COLUMN turn_sequence INTEGER;
ALTER TABLE invocations ADD COLUMN parallel_group INTEGER;
```

| Column | Type | Purpose |
|--------|------|---------|
| `turn_id` | TEXT | Groups invocations within a turn. Hash of (session_id, turn_start_timestamp). NULL for hook-ingested records (no transcript parsed). |
| `turn_sequence` | INTEGER | Ordering within the turn (0-indexed). Parallel calls share the same sequence number. |
| `parallel_group` | INTEGER | Distinguishes parallel calls at the same sequence. All calls in the same parallel batch get the same `turn_sequence` but different `parallel_group`. Solo calls get `parallel_group = 0`. |

**Pros**: No new tables, no joins. `GROUP BY turn_id` gives turn-level
aggregation. Existing queries still work (new columns are nullable).

**Cons**: Denormalized. Turn-level metadata (duration, start time) would need
a separate query or a new table.

### Option B: Separate turns table

```sql
CREATE TABLE turns (
    id            TEXT PRIMARY KEY,
    session_id    TEXT NOT NULL,
    turn_index    INTEGER NOT NULL,
    started_at    TEXT NOT NULL,
    duration_ms   INTEGER,
    tool_count    INTEGER,
    error_count   INTEGER,
    was_compacted BOOLEAN DEFAULT FALSE
);

-- Add FK on invocations
ALTER TABLE invocations ADD COLUMN turn_id TEXT REFERENCES turns(id);
ALTER TABLE invocations ADD COLUMN turn_sequence INTEGER;
```

**Pros**: Clean normalization. Turn-level queries are direct. Can store
per-turn metadata (duration, compaction status).

**Cons**: Extra table, extra join. Invocation queries need `LEFT JOIN` for
turn context.

### Recommendation: Option A

The primary consumer is `dp paths` and `dp suggest`, which aggregate by
tool_name. Turn context is supplementary — "this tool is usually called 3rd
in a 5-tool turn" is useful but secondary to "this tool is called N times."
Option A keeps the common path fast (no join) and adds turn context as
enrichment.

If turn-level analytics become a primary use case later, we can add a
materialized turns view or promote to Option B.

### Model Changes

```go
type Invocation struct {
    // ... existing fields ...
    TurnID        string `json:"turn_id,omitempty"`
    TurnSequence  *int   `json:"turn_sequence,omitempty"`
    ParallelGroup *int   `json:"parallel_group,omitempty"`
}
```

## Implementation Plan

### Phase 1: Transcript parser (library only)

New package: `internal/transcript/`

```go
// Parse reads a Claude Code transcript JSONL and returns structured turns.
func Parse(r io.Reader) ([]Turn, error)

type Turn struct {
    SessionID   string
    Index       int           // 0-based turn number in session
    StartedAt   time.Time
    DurationMs  int           // from turn_duration system event, 0 if absent
    UserPrompt  string        // human text that started this turn
    Invocations []Invocation  // tool calls in order
    WasCompacted bool         // true if a compact_boundary precedes this turn
}

type Invocation struct {
    ToolName      string
    ToolUseID     string
    Input         json.RawMessage
    Sequence      int  // position in turn
    ParallelGroup int  // 0 = solo, >0 = parallel batch
    IsError       bool
    Error         string
    DurationMs    int  // if measurable from timestamps
}
```

**Parsing algorithm:**

1. Read all JSONL events into memory (transcripts are <10MB typically).
2. Build UUID→event index.
3. Walk events in timestamp order.
4. Identify turn starts: `user` events with string content (not tool_result).
5. For each turn, walk the `parentUuid` tree to collect all descendant events.
6. Within a turn, identify tool_use assistant events and their corresponding
   tool_result user events (matched via `sourceToolAssistantUUID`).
7. Determine sequence: timestamp order of tool_use events within the turn.
8. Detect parallel groups: tool_use events that share the same parent
   (chained from the same text/thinking block without intervening tool_results)
   are parallel.
9. Turn ends at `stop_hook_summary` system event.

### Phase 2: Schema migration + enrichment

- Add V3 migration with `turn_id`, `turn_sequence`, `parallel_group` columns.
- New CLI command: `dp enrich --transcript <path>` that:
  1. Parses the transcript.
  2. Matches invocations by `(session_id, tool_use_id)` — both already stored.
  3. Updates `turn_id`, `turn_sequence`, `parallel_group` on matched rows.
- Alternatively, `dp ingest --transcript <path>` as a new ingest mode that
  creates invocations with turn data populated from the start.

### Phase 3: Turn-aware queries

- `dp paths --turns` shows average turn position for each tool pattern.
- `dp inspect <tool> --turns` shows which tools typically precede/follow it.
- `dp suggest` uses turn context to improve recommendations (e.g., "this tool
  is always called after Glob — consider aliasing the pair").

### Phase 4: Automatic enrichment (optional)

- PostToolUse hook already receives `transcript_path`. After ingesting the
  invocation, schedule a background enrichment pass on the transcript.
- Or: SessionStart hook triggers a full-session parse of the previous
  session's transcript.

## Open Questions

1. **Subagent transcripts**: Should subagent tool calls be tracked as part of
   the parent turn? They're in separate files but share the `sessionId`.
   Recommendation: Yes, with a `is_subagent` flag. The parent Task tool_use
   gives the parent turn context.

2. **Compaction**: After compaction, the conversation tree is broken. Should
   we mark post-compaction turns specially? Recommendation: Yes, `was_compacted`
   flag on the turn. Sequence numbers reset.

3. **Transcript availability**: `transcript_path` is only available during the
   session. After session end, the file is at a known path
   (`~/.claude/projects/<slug>/<session-id>.jsonl`). Should `dp enrich` find
   transcripts automatically by session_id? Recommendation: Yes, with
   `--claude-dir` flag defaulting to `~/.claude`.

4. **Privacy**: Transcripts contain full conversation content (user prompts,
   tool outputs). The parser should extract only structural metadata (tool
   names, sequence, timing) — never store prompt text or tool output in dp's
   database.

## Files Modified

| File | Changes |
|------|---------|
| `internal/transcript/parse.go` | **New**: JSONL parser, turn extraction |
| `internal/transcript/parse_test.go` | **New**: Unit tests with fixture transcripts |
| `internal/transcript/testdata/` | **New**: Minimal test fixtures |
| `internal/model/model.go` | Add TurnID, TurnSequence, ParallelGroup to Invocation |
| `internal/store/sqlite.go` | V3 migration, updated insert/query |
| `internal/cli/enrich.go` | **New**: `dp enrich` command |

## Verification

1. `go test ./internal/transcript/` — parser correctness.
2. `dp enrich --transcript <real-transcript>` — matches invocations, populates turn data.
3. `dp paths` — still works (nullable columns, no regression).
4. `dp inspect <tool> --turns` — shows turn position data.
