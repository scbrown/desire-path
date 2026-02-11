# 009 - New dp suggest: Data-Driven Suggestions from Collected Desires

## Problem

`dp suggest` today is just string similarity matching. Given `read_file`, it
compares against a hardcoded list of known tools (Read, Write, Edit, ...) and
returns the closest Levenshtein match. It doesn't look at the database at all.

This is useful for one narrow case: "I have a hallucinated name, what's the
real tool?" But dp is sitting on a database of desires (failures), invocations
(all calls), aliases (configured corrections), and soon turn data. A smarter
`dp suggest` should mine all of that to produce actionable recommendations —
not just "did you mean Read?" but "here are the 5 things you should do to
improve your agent's tool use."

## Prerequisites

- **dp-rhb**: Rename current `dp suggest` to `dp similar`. The string
  similarity matcher is still useful but it's a building block, not the
  top-level command.
- **dp-6yn** (closed): pave-check records corrections as desires, so aliased
  patterns stay visible to analytics.
- **dp-ipz** (closed): env-need categorization exists, so missing-tool
  desires are classified.

## What dp suggest Should Do

`dp suggest` with no arguments analyzes the full database and outputs a
prioritized list of actionable suggestions. Each suggestion has a type, a
recommendation, and evidence.

### Suggestion Types

#### 1. Unaliased Hallucinations

**Signal**: Desire paths (failed tool names) with no alias configured.
**Query**: `GetPaths()` filtered to entries where `AliasTo` is empty.
**Output**:
```
PRIORITY  TYPE          PATTERN       COUNT  SUGGESTION
1         hallucination read_file     42     dp alias read_file Read (similar: Read 0.85)
2         hallucination write_file    28     dp alias write_file Write (similar: Write 0.87)
3         hallucination search_files  15     dp alias search_files Grep (similar: Grep 0.62)
```

Uses `analyze.SuggestN()` (the existing similarity matcher, soon renamed to
`dp similar`) to propose the alias target. If similarity is low, suggests
manual review instead of a specific alias.

#### 2. Env Needs (Missing Commands)

**Signal**: Desires with `category = "env-need"`.
**Query**: `ListDesires(opts)` with `Category: "env-need"`, deduplicated by
extracted command name.
**Output**:
```
PRIORITY  TYPE          PATTERN              COUNT  SUGGESTION
4         env-need      cargo-insta          8      Install cargo-insta: cargo install cargo-insta
5         env-need      cargo-nextest        5      Install cargo-nextest: cargo install cargo-nextest
```

Uses `analyze.EnvNeedCommand()` to extract the missing command. Where
possible, suggests the install command (a lookup table for common tools:
`cargo install X`, `npm install -g X`, `pip install X`, `brew install X`).
Falls back to "Install <command>" for unknown package managers.

#### 3. High-Frequency Error Tools

**Signal**: Tools (real, not hallucinated) with high error rates in
invocations.
**Query**: Compare `InvocationStats()` per tool — tools where
`error_count / total_count > threshold` (e.g., > 30%).
**Output**:
```
PRIORITY  TYPE          PATTERN       ERRORS/TOTAL  SUGGESTION
6         error-prone   Bash          45/120 (38%)  Review Bash errors: dp inspect Bash
7         error-prone   Edit          12/89 (13%)   Review Edit errors: dp inspect Edit
```

The suggestion here isn't a fix — it's a pointer to `dp inspect` for manual
investigation. High error rates on real tools indicate the agent is misusing
them (wrong flags, wrong arguments, wrong expectations).

#### 4. Correction Candidates (from pave-check data)

**Signal**: Desires with `source = "pave-check"` or from correction events
(dp-6yn). These are tool calls that were already caught and corrected but
represent ongoing friction.
**Query**: Desires created by pave-check, grouped by pattern.
**Output**:
```
PRIORITY  TYPE          PATTERN       COUNT  SUGGESTION
8         still-firing  read_file     12     Alias installed but still triggered 12 times. Consider AGENTS.md rule.
```

If a pave-check correction fires repeatedly, the alias alone isn't enough —
the agent keeps hallucinating the same tool. Suggests adding an AGENTS.md
rule via `dp pave --agents-md` to teach the agent proactively rather than
just catching mistakes.

### Prioritization

Suggestions are ranked by **actionability × frequency**:

1. Unaliased hallucinations (most actionable — one command fixes it)
2. Env needs (actionable — install the tool)
3. Still-firing corrections (alias exists but agent isn't learning)
4. Error-prone tools (needs investigation, not a quick fix)

Within each type, sorted by count descending.

### CLI Interface

```
dp suggest                          # all suggestions
dp suggest --type hallucination     # filter by type
dp suggest --type env-need
dp suggest --min-count 5            # only patterns seen 5+ times
dp suggest --json                   # machine-readable output
dp suggest --apply                  # interactively apply suggestions (future)
```

No positional argument. The old `dp suggest <tool-name>` behavior moves to
`dp similar <tool-name>`.

### JSON Output

```json
{
  "suggestions": [
    {
      "priority": 1,
      "type": "hallucination",
      "pattern": "read_file",
      "count": 42,
      "suggestion": "dp alias read_file Read",
      "evidence": {
        "similar_tool": "Read",
        "similarity_score": 0.85,
        "first_seen": "2026-01-15T10:30:00Z",
        "last_seen": "2026-02-11T15:45:00Z"
      }
    }
  ]
}
```

## Implementation

### Package: `internal/analyze/recommendations.go`

New file alongside the existing `suggest.go` (similarity) and
`categorize.go` (env-need detection).

```go
// RecommendationType identifies what kind of suggestion this is.
type RecommendationType string

const (
    RecHallucination  RecommendationType = "hallucination"
    RecEnvNeed        RecommendationType = "env-need"
    RecErrorProne     RecommendationType = "error-prone"
    RecStillFiring    RecommendationType = "still-firing"
)

// Recommendation is a single actionable suggestion.
type Recommendation struct {
    Priority   int                `json:"priority"`
    Type       RecommendationType `json:"type"`
    Pattern    string             `json:"pattern"`
    Count      int                `json:"count"`
    Suggestion string             `json:"suggestion"`
    Evidence   map[string]any     `json:"evidence"`
}

// Recommend analyzes the store and returns prioritized suggestions.
func Recommend(ctx context.Context, s store.Store, opts RecommendOpts) ([]Recommendation, error)

type RecommendOpts struct {
    MinCount  int
    TypeFilter RecommendationType // empty = all
    Since     time.Time
}
```

Each suggestion type is a separate function:
- `findUnaliasedHallucinations(ctx, s) → []Recommendation`
- `findEnvNeeds(ctx, s) → []Recommendation`
- `findErrorProneTools(ctx, s) → []Recommendation`
- `findStillFiringCorrections(ctx, s) → []Recommendation`

`Recommend()` calls all four, merges, assigns priorities, and sorts.

### CLI: `internal/cli/suggest.go` (rewritten)

After dp-rhb renames the old suggest to similar, this file gets rewritten
to call `analyze.Recommend()`.

### Store Additions

May need a new query method:

```go
// ToolErrorRates returns per-tool error counts and totals from invocations.
ToolErrorRates(ctx context.Context, opts InvocationOpts) ([]ToolErrorRate, error)

type ToolErrorRate struct {
    ToolName   string
    TotalCount int
    ErrorCount int
    ErrorRate  float64
}
```

The rest (GetPaths, ListDesires with category filter, GetAliases) already
exists.

### Install Command Lookup

A small table mapping common commands to their install instructions:

```go
var installHints = map[string]string{
    "cargo-insta":    "cargo install cargo-insta",
    "cargo-nextest":  "cargo install cargo-nextest",
    "rg":             "cargo install ripgrep",
    "fd":             "cargo install fd-find",
    "jq":             "apt install jq / brew install jq",
    "yq":             "pip install yq / brew install yq",
    // ... extensible
}
```

Not comprehensive — just the common ones. Falls back to "Install <command>"
for unknowns. Could later be made configurable or community-contributed.

## Dependency Chain

```
dp-rhb  Rename dp suggest → dp similar
  ↓
dp-0ac  New dp suggest (this plan)
```

dp-0ac should be split into:
1. `analyze.Recommend()` + unit tests (core logic)
2. CLI rewrite + store additions (wiring)

But since dp-0ac already exists and is well-scoped, it can stay as a
single bead unless it grows.

## Files Modified

| File | Changes |
|------|---------|
| `internal/analyze/recommendations.go` | **New**: Recommend(), suggestion type functions |
| `internal/analyze/recommendations_test.go` | **New**: Unit tests for each suggestion type |
| `internal/analyze/install_hints.go` | **New**: Install command lookup table |
| `internal/cli/suggest.go` | Rewritten to use analyze.Recommend() |
| `internal/store/store.go` | Add ToolErrorRates() method |
| `internal/store/sqlite.go` | Implement ToolErrorRates() |

## Future Extensions

- **`dp suggest --apply`**: Interactive mode that asks "Apply this alias? [y/n]"
  and runs the command. Batch alias creation.
- **Turn-pattern suggestions**: Once dp-bbn lands, add a 5th suggestion type
  based on recurring long turns (e.g., "Grep → Read{3+} → Edit appears 12
  times — consider a recipe alias").
- **Cross-session trends**: "read_file failures increased 300% this week" —
  temporal trend detection.
- **Custom known tools**: Instead of hardcoding Claude Code tools, read the
  tool list from config or from invocation data (tools that succeeded are
  "known").
