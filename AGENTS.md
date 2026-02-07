# Desire Path (dp) - Agent Instructions

Read this file first when working on this codebase. It covers project conventions, Go idioms, and architectural decisions that all AI agents should follow.

## Project Overview

`dp` is a Go CLI that collects, analyzes, and surfaces patterns from failed AI tool calls ("desires"). Failed tool calls from AI coding assistants are signals - they reveal capabilities the AI expects to exist. By tracking these, developers can implement new features or aliases so future similar attempts succeed.

See `docs/plans/001-initial-plan.md` for the full architecture and design.

## Project Layout

```
cmd/dp/          Entry point. Thin main.go that calls into internal/cli.
internal/        Private packages - not importable by external code.
  model/         Core types: Desire, Path, Alias.
  store/         Storage interface + SQLite implementation.
  record/        Stdin JSON parsing and desire recording.
  analyze/       Similarity engine for tool name suggestions.
  config/        Configuration file (~/.dp/config.json) management.
  cli/           Cobra command definitions + table formatting.
docs/plans/      Architecture and design documents.
docs/tasks/      Task breakdowns for implementation phases.
```

### internal/ packages

- `internal/` is for packages only used by `dp` itself. Go enforces this boundary.
- `pkg/desirepath/` is planned as a public Go library for programmatic integration but is not yet implemented.

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

- Go 1.24+ (see `go.mod` for exact version)
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
- Subcommands registered via `init()` in each command file
- Use `RunE` (not `Run`) so errors propagate properly
- Read stdin with `os.Stdin` - don't assume TTY

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

`dp config` manages settings in `~/.dp/config.json` via the `internal/config` package. Valid keys: `db_path`, `default_source`, `known_tools`, `default_format`. The root command's `PersistentPreRun` loads config and applies defaults for `--db` and `--json` flags.

## SQLite Conventions

- Use `modernc.org/sqlite` (pure Go, no CGo) - import as `_ "modernc.org/sqlite"`
- Open with `database/sql` stdlib interface
- Schema migrations: simple version table + sequential SQL statements
- Use `?` parameter placeholders (not `$1`)
- Always `defer rows.Close()` after query
- Wrap multi-statement writes in transactions

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
