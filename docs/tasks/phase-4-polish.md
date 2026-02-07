# Phase 4: Polish

Output quality, machine-readable output, configuration, and test coverage.

## Tasks

### 4.1 Table formatting
- [ ] Consistent column-aligned table output across all commands
- [ ] Auto-detect terminal width and truncate/wrap accordingly
- [ ] Use stdlib `text/tabwriter` (no external table library)
- [ ] Color output for TTY, plain for pipes (detect with `os.Stdout.Fd()` + `isatty`)

### 4.2 JSON output mode
- [ ] `--json` global flag on root command
- [ ] All commands that produce output support `--json`
- [ ] JSON output writes to stdout, human-readable to stderr when mixed
- [ ] Structured output suitable for `jq` piping

### 4.3 Config file support
- [ ] Config location: `~/.dp/config.json`
- [ ] `dp config` - Show current config
- [ ] `dp config <key> <value>` - Set a value
- [ ] `dp config <key>` - Get a value
- [ ] Configurable settings:
  - `db_path` - Override default database location
  - `default_source` - Default `--source` value
  - `known_tools` - List of known tool names for suggestions
  - `default_format` - Default export format

### 4.4 Test coverage
- [ ] Unit tests for all `internal/` packages
- [ ] Integration test: record → list → paths round-trip
- [ ] Edge cases: empty DB, malformed JSON, missing fields, very large inputs
- [ ] `go test -race ./...` passes
- [ ] `go vet ./...` clean

### 4.5 Build & distribution
- [ ] `Makefile` with targets: build, test, vet, clean, install
- [ ] `go install` works: `go install github.com/<owner>/dp/cmd/dp@latest`
- [ ] `.goreleaser.yml` for cross-platform binary releases (future)
- [ ] `.gitignore` for build artifacts

### 4.6 Documentation
- [ ] Update AGENTS.md with any new conventions discovered during implementation
- [ ] CLI help text: `Short` and `Long` descriptions on every command
- [ ] `Example` field on commands with non-obvious usage
- [ ] Update plan doc with any architectural changes made during implementation

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
