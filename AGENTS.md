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
  analyze/       Path aggregation and similarity suggestions.
  config/        Configuration file load/save (~/.dp/config.json).
  cli/           Cobra command definitions.
pkg/desirepath/  Public Go library for programmatic integration.
docs/plans/      Architecture and design documents.
docs/tasks/      Task breakdowns for implementation phases.
```

### internal/ vs pkg/

- `internal/` is for packages only used by `dp` itself. Go enforces this boundary.
- `pkg/desirepath/` is the public API for other Go programs to integrate with Desire Path programmatically (e.g., recording desires from their own code).

## Documentation Hygiene

Documentation is not an afterthought - it is part of the work. When you build new functionality, document it before moving on.

- **AGENTS.md**: Update this file when adding new packages, changing conventions, or adding dependencies.
- **docs/tasks/**: Update task status when starting or completing work.
- **Code comments**: Exported symbols get doc comments. Write them as you write the code, not later.
- **CLI help text**: Every cobra command must have a `Short` and `Long` description. Include examples in `Example` field for non-trivial commands.
- **docs/plans/**: When proposing architectural changes, write a plan document before implementing.

Stale or missing documentation is a bug. If you notice something undocumented, fix it.

## Go Conventions

### General

- Go 1.24+ (use modern idioms: range-over-int, structured logging)
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
| `golang.org/x/term` | Terminal detection and width |

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
| `known_tools` | Comma-separated known tool names for `dp suggest` |
| `default_format` | Default output format: `table` or `json` |

Config values are loaded in `PersistentPreRun` on the root command, so they apply
to all subcommands unless overridden by flags.

## Build & Run

```bash
make build                     # Build binary (outputs ./dp)
make test                      # Test all packages
make vet                       # Static analysis
make install                   # Install to $GOPATH/bin
make clean                     # Remove build artifacts
echo '{}' | ./dp record       # Quick smoke test
```
