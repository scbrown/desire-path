# Phase 3: Suggestions & Aliases

Smart features that turn collected data into actionable mappings.

## Tasks

### 3.1 Similarity engine (`internal/analyze/suggest.go`)
- [ ] `Suggest(name string, known []string) []Suggestion` function
- [ ] Levenshtein distance scoring (normalized to 0.0-1.0)
- [ ] Common prefix/suffix bonus (e.g., `read_file` ~ `Read` via "read" prefix)
- [ ] Case-insensitive and underscore/camelCase normalization
- [ ] Return top N suggestions above a threshold (default 0.5)
- [ ] `Suggestion` struct: `{Name string, Score float64}`

### 3.2 Suggest command (`internal/cli/suggest.go`)
- [ ] `dp suggest <tool-name>` - Suggest existing tool mappings
- [ ] Accept a `--known` flag or read known tools from config/aliases
- [ ] Default known tools: Claude Code built-in tools (Read, Write, Edit, Bash, Glob, Grep, etc.)
- [ ] Output: ranked suggestions with scores
- [ ] If exact alias exists, show it

### 3.3 Alias command (`internal/cli/alias.go`)
- [ ] `dp alias <from> <to>` - Create a mapping
- [ ] `dp aliases` - List all aliases
- [ ] `dp alias --delete <from>` - Remove an alias
- [ ] Aliases stored in SQLite `aliases` table
- [ ] Prevent duplicate aliases (upsert behavior)

### 3.4 Init command (`internal/cli/init_cmd.go`)
- [ ] `dp init` - Interactive setup wizard
- [ ] `dp init --claude-code` - Write Claude Code hook config
  - [ ] Detect `~/.claude/settings.json` existence
  - [ ] Merge PostToolUseFailure hook into existing config
  - [ ] Don't clobber existing hooks
  - [ ] Print confirmation with instructions
- [ ] Future: `--cursor`, `--copilot` flags for other tools

## Done when

```bash
./dp suggest read_file
# Suggestion: "read_file" is similar to "Read" (score: 0.82)

./dp alias read_file Read
./dp aliases
# read_file → Read

./dp init --claude-code
# Hook configuration written to ~/.claude/settings.json
```

## Depends on

- Phase 1 (Core)
- Phase 2 is NOT required (these can be built in parallel)

## Blocks

- Nothing directly, but aliases enhance the paths display (Phase 4)
