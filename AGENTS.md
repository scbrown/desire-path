# Desire Path (dp) - Agent Instructions

Read this file first when working on this codebase. It covers project conventions, Go idioms, and architectural decisions that all AI agents should follow.

## Project Overview

`dp` is a Go CLI that collects, analyzes, and surfaces patterns from failed AI tool calls ("desires"). Failed tool calls from AI coding assistants are signals - they reveal capabilities the AI expects to exist. By tracking these, developers can implement new features or aliases so future similar attempts succeed.

See `docs/plans/001-initial-plan.md` for the full architecture and design.

## Project Layout

```
cmd/dp/          Entry point. Thin main.go that calls into internal/cli.
internal/        Private packages - not importable by external code.
  model/         Core types: Desire, Path, Alias, Invocation.
  store/         Storage interface + SQLite implementation.
  source/        Source plugin interface, registry, and built-in plugins.
  ingest/        Raw payload → Invocation conversion and persistence.
  record/        Stdin JSON parsing and desire recording.
  analyze/       Similarity engine for tool name suggestions.
  signpost/      PostToolUse gating, intent discovery, stack pointer/payload
                 emission, eval JSONL.
  eval/          Ground-truth validation and blocked assignment matrices.
  config/        Configuration file (~/.dp/config.toml) management.
  cli/           Cobra command definitions + table formatting.
docs/plans/      Architecture and design documents.
docs/tasks/      Task breakdowns for implementation phases.
```

### internal/ packages

- `internal/` is for packages only used by `dp` itself. Go enforces this boundary.
- `pkg/desirepath/` is planned as a public Go library for programmatic integration but is not yet implemented.

#### source

Defines the `Source` plugin interface for extracting structured fields from raw AI tool call payloads. Each plugin handles one tool's format (e.g., Claude Code). The package provides a thread-safe registry (`Register`, `Get`, `Names`) and the universal `Fields` struct that normalizes data across sources. Plugins self-register via `init()`. The optional `Installer` interface lets plugins provide setup logic for `dp init`. See [Source Plugins](#source-plugins) below.

#### ingest

Bridges source extraction and storage. The `Ingest` function looks up a registered source plugin by name, calls `Extract` on raw bytes, converts the resulting `Fields` into a `model.Invocation` (auto-generating UUID and timestamp), and persists it via `store.RecordInvocation`. This is the single entry point for recording tool call data from any source.

#### signpost

Observes completed Bash searches on PostToolUse and, when the result is weak,
offers the stack instead. Two things it must never do: modify the tool call, or
take longer than the 150 ms hook contract. Both are structural — the hook only
ever emits `additionalContext`, and every failure path returns silence.

**Intent families.** `DiscoverIntent` parses one observed command into an
`Intent`. There are four families, each a parser plus a predicate plus its own
evaluation row:

| family | trigger | stack command | route |
|---|---|---|---|
| `literal-search` | `grep`/`rg` PATTERN | `bobbin search` | `/search` |
| `file-find` | `find … -name PAT` | `bobbin search` | `/search` |
| `history-search` | `git log -S`/`-G`/`--grep` | `bobbin search --type commit` | `/search?type=commit` |
| `symbol-search` | a bare identifier that RESOLVES | `yupana callers` | `/refs` |

`symbol-search` is promoted by **resolution, not by the parse** — an identifier
that resolves nowhere is just a string and is searched as one. That fallback is
a second round trip, so it is skipped when the remaining budget cannot pay for
it: the contract outranks the upgrade. It is also the one family whose check and
whose pointer come from different tools (bobbin resolves it, yupana is pointed
at), which is stated in the code rather than hidden.

**Pointer vs payload.** By default a signpost names a COMMAND. Under a payload
condition (or `DP_SIGNPOST_PAYLOAD=1`) it injects the RESULT instead — top-N
locations with one-line snippets, size-capped. A count with no locations
degrades to a pointer rather than emitting an empty answer, which would read to
the model as a failed search.

The two arms' adoption metrics are **not the same measurement** — a pointer is
adopted by being RUN, a payload by being USED — so they are planned separately
(`dp eval plan --arm payload`), every result row carries `adoption_kind`, and
`eval.Score` refuses to aggregate a cell whose rows disagree about the kind.

**Hook output MUST name its own event.** `hookSpecificOutput.hookEventName`
has to match the event the payload came from. Output whose name does not match
is DISCARDED, silently — nothing reports the rejection. This cost weeks: the
signpost hardcoded `PostToolUse`, so once it was also registered on
PostToolUseFailure its context was dropped there, and `dp pave-correct` emitted
no name at all and was dropped on every failure it ever handled. Both are fixed
and both are asserted by tests, because the harness will not tell you.

**The prefetch resolves the family.** `signpost-prefetch` runs on PreToolUse
with a 5 s budget and writes the resolved family and hits into the warm cache;
PostToolUse adopts that resolution. This is the only way the symbol family can be
delivered inside 150 ms, and it is why `CacheKey` is keyed on repo and query but
NOT on family — the family is an output of the lookup, not an input to it.

## Documentation Hygiene

Documentation is not an afterthought - it is part of the work. When you build new functionality, document it before moving on.

- **AGENTS.md**: Update this file when adding new packages, changing conventions, or adding dependencies.
- **docs/tasks/**: Update task status when starting or completing work.
- **Code comments**: Exported symbols get doc comments. Write them as you write the code, not later.
- **CLI help text**: Every cobra command must have `Short`, `Long`, and `Example` fields. `Short` is a single-line summary. `Long` explains behavior, flags, and edge cases. `Example` shows practical usage with realistic arguments.
- **docs/plans/**: When proposing architectural changes, write a plan document before implementing.

Stale or missing documentation is a bug. If you notice something undocumented, fix it.

## Go Conventions

### General

- Go 1.24+ (see `go.mod` for exact version; use modern idioms: range-over-int, structured logging)
- Format with `gofmt`. No exceptions.
- Packages are named short, lowercase, singular: `store`, `model`, `record`
- Exported names get doc comments. Unexported names get comments only when non-obvious.

### Error Handling

```go
// Wrap errors with context using fmt.Errorf and %w.
result, err := store.GetDesire(id)
if err != nil {
    return fmt.Errorf("getting desire %s: %w", id, err)
}
```

- Never ignore errors silently. Either handle, return, or explicitly comment why it's safe to discard.
- Use `errors.Is` / `errors.As` for checking error types, not string matching.
- Define sentinel errors in the package that produces them: `var ErrNotFound = errors.New("not found")`

### Naming

- Interfaces: verb-er suffix (`Store`, `Reader`, `Recorder`) or describe capability
- Constructors: `NewStore(...)`, `NewRecorder(...)`
- Boolean vars/fields: `isReady`, `hasAlias` (or just descriptive: `ready`, `aliased`)
- Avoid stuttering: `store.New()` not `store.NewStore()`

### Testing

- Use stdlib `testing` package. No testify, no gomega.
- Table-driven tests for anything with multiple cases:

```go
func TestSomething(t *testing.T) {
    tests := []struct {
        name string
        input string
        want  string
    }{
        {"empty input", "", ""},
        {"basic case", "foo", "bar"},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := doSomething(tt.input)
            if got != tt.want {
                t.Errorf("got %q, want %q", got, tt.want)
            }
        })
    }
}
```

- Test files live next to the code they test: `store/sqlite_test.go`
- Use `t.TempDir()` for any file/DB operations in tests.

## Dependencies

Keep dependencies minimal:

| Package | Purpose |
|---------|---------|
| `github.com/spf13/cobra` | CLI framework |
| `modernc.org/sqlite` | Pure Go SQLite (no CGo required) |
| `github.com/google/uuid` | UUID generation |
| `golang.org/x/term` | Terminal detection and width measurement |

The Levenshtein distance algorithm for tool name similarity is implemented in `internal/analyze/suggest.go` rather than using an external dependency.

Do not add new dependencies without strong justification.

## CLI Patterns (Cobra)

- Each subcommand in its own file under `internal/cli/`
- Root command defined in `root.go` with global flags (`--db`, `--json`)
- Subcommands registered via `rootCmd.AddCommand()` in each file's `init()`
- Use `RunE` (not `Run`) so errors propagate properly
- Read stdin with `os.Stdin` - don't assume TTY
- Table output uses `internal/cli.Table` (wraps `text/tabwriter`) with TTY-aware color and terminal width
- All commands that produce output support `--json` for machine-readable output
- JSON output writes to stdout; human-readable status messages go to stderr

### Table Output

All commands producing tabular output use the `Table` type from `internal/cli/table.go`:

```go
tbl := NewTable(os.Stdout, "COLUMN1", "COLUMN2")
tbl.Row("value1", "value2")
tbl.Flush()
```

- Headers are bold when output is a TTY, plain when piped
- Terminal width auto-detected; defaults to 80 when not a TTY
- Long values can be truncated with `truncate(s, maxLen)`
- Use `tbl.Color()` to check if color is enabled before adding ANSI codes

### JSON Output

All output commands support `--json` (global flag on root). Check `jsonOutput` and emit JSON before any table rendering:

```go
if jsonOutput {
    enc := json.NewEncoder(os.Stdout)
    enc.SetIndent("", "  ")
    return enc.Encode(data)
}
```

The `default_format` config key can set JSON as default output; the `--json` flag always overrides.

### Configuration

`dp config` manages settings in `~/.dp/config.toml` via the `internal/config` package. Valid keys: `db_path`, `default_source`, `known_tools`, `default_format`. The root command's `PersistentPreRun` loads config and applies defaults for `--db` and `--json` flags.

## Source Plugins

The `internal/source` package uses a plugin architecture for extracting structured data from different AI tools. Each tool (Claude Code, Cursor, etc.) has its own plugin that knows how to parse that tool's raw payload format.

### Writing a Source Plugin

1. **Implement `source.Source`**: provide `Name()` (lowercase, hyphenated identifier like `"claude-code"`) and `Extract(raw []byte) (*Fields, error)`.
2. **Self-register in `init()`**: call `source.Register(&yourPlugin{})` so the plugin is available as soon as the package is imported.
3. **Map to universal `Fields`**: extract `ToolName` (required), `InstanceID`, `ToolInput`, `CWD`, and `Error` into the struct fields. Put everything else into `Fields.Extra`.
4. **Import for side effects**: add a blank import (`_ "github.com/scbrown/desire-path/internal/source"`) where plugins need to be loaded (e.g., in `cmd/dp/`).

### Installer Interface (Optional)

Plugins that need setup during `dp init` implement the `Installer` interface:

```go
type Installer interface {
    Install(settingsPath string) error
}
```

Conventions for `Install` implementations:

- Accept `settingsPath` as parameter; use a sensible default (e.g., `~/.claude/settings.json`) when empty.
- Read existing settings and merge — never clobber user config.
- Be idempotent: check whether the hook/config already exists before adding.
- Create parent directories as needed (`os.MkdirAll`).
- Wrap errors with context (`fmt.Errorf("...: %w", err)`).

The `dp init` command checks each registered source with a type assertion (`s.(source.Installer)`) and calls `Install` for plugins that support it.

### Data Flow

```
Raw hook payload (JSON bytes)
  → source.Source.Extract()     → source.Fields (universal)
  → ingest.Ingest()             → model.Invocation (UUID, timestamp added)
  → store.RecordInvocation()    → SQLite
```

## SQLite Conventions

- Use `modernc.org/sqlite` (pure Go, no CGo) - import as `_ "modernc.org/sqlite"`
- Open with `database/sql` stdlib interface
- Schema migrations: simple version table + sequential SQL statements
- Use `?` parameter placeholders (not `$1`)
- Always `defer rows.Close()` after query
- Wrap multi-statement writes in transactions

## Configuration

`dp` uses `~/.dp/config.json` for persistent settings, managed via `dp config`.

| Key | Purpose |
|-----|---------|
| `db_path` | Override default database location |
| `default_source` | Default `--source` value for `dp record` |
| `known_tools` | Comma-separated known tool names for `dp similar` |
| `default_format` | Default output format: `table` or `json` |

Config values are loaded in `PersistentPreRun` on the root command, so they apply
to all subcommands unless overridden by flags.

## Build & Run

```bash
make build                     # Build binary (./dp)
make test                      # Run all tests
make vet                       # Static analysis
make install                   # Install to $GOPATH/bin
make clean                     # Remove build artifacts
echo '{}' | ./dp record       # Quick smoke test
```

Cross-platform releases are configured via `.goreleaser.yml` (linux, darwin, windows; amd64, arm64).
