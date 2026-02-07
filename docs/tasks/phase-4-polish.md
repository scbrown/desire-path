# Phase 4: Polish

Output quality, machine-readable output, configuration, and test coverage.

## Tasks

### 4.1 Table formatting
- [x] Consistent column-aligned table output across all commands
- [x] Auto-detect terminal width and truncate/wrap accordingly
- [x] Use stdlib `text/tabwriter` (no external table library)
- [x] Color output for TTY, plain for pipes (detect with `golang.org/x/term`)

### 4.2 JSON output mode
- [x] `--json` global flag on root command
- [x] All commands that produce output support `--json`
- [x] JSON output writes to stdout, human-readable to stderr when mixed
- [x] Structured output suitable for `jq` piping

### 4.3 Config file support
- [x] Config location: `~/.dp/config.toml`
- [x] `dp config` - Show current config
- [x] `dp config <key> <value>` - Set a value
- [x] `dp config <key>` - Get a value
- [x] Configurable settings:
  - `db_path` - Override default database location
  - `default_source` - Default `--source` value
  - `known_tools` - List of known tool names for suggestions
  - `default_format` - Default export format

### 4.4 Test coverage
- [x] Unit tests for all `internal/` packages
- [x] Integration test: record → list → paths round-trip
- [x] Edge cases: empty DB, malformed JSON, missing fields, very large inputs
- [x] `go test -race ./...` passes
- [x] `go vet ./...` clean

### 4.5 Build & distribution
- [x] `Makefile` with targets: build, test, vet, clean, install
- [x] `go install` works: `go install github.com/<owner>/dp/cmd/dp@latest`
- [x] `.goreleaser.yml` for cross-platform binary releases (future)
- [x] `.gitignore` for build artifacts

### 4.6 Documentation
- [x] Update AGENTS.md with any new conventions discovered during implementation
- [x] CLI help text: `Short` and `Long` descriptions on every command
- [x] `Example` field on commands with non-obvious usage
- [x] Update plan doc with any architectural changes made during implementation

## Done when

```bash
./dp list --json | jq '.[] | .tool_name'
# Produces valid JSON

./dp config db_path
# ~/.dp/desires.db

go test -race ./...
# All pass

go vet ./...
# Clean
```

## Depends on

- Phase 1 (Core)
- Phase 2 (Reporting) - for table formatting improvements
- Phase 3 (Suggestions) - for alias display in paths

## Blocks

- Nothing - this is the final phase before v1.0
