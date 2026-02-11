# Phase 3: Suggestions & Aliases

Smart features that turn collected data into actionable mappings.

## Tasks

### 3.1 Similarity engine (`internal/analyze/suggest.go`) — used by `dp similar`
- [x] `Suggest(name string, known []string) []Suggestion` function
- [x] Levenshtein distance scoring (normalized to 0.0-1.0)
- [x] Common prefix/suffix bonus (e.g., `read_file` ~ `Read` via "read" prefix)
- [x] Case-insensitive and underscore/camelCase normalization
- [x] Return top N suggestions above a threshold (default 0.5)
- [x] `Suggestion` struct: `{Name string, Score float64}`

### 3.2 Similar command (`internal/cli/similar.go`)
- [x] `dp similar <tool-name>` - Find known tools similar to a tool name
- [x] Accept a `--known` flag or read known tools from config/aliases
- [x] Default known tools: Claude Code built-in tools (Read, Write, Edit, Bash, Glob, Grep, etc.)
- [x] Output: ranked suggestions with scores
- [x] If exact alias exists, show it

### 3.3 Alias command (`internal/cli/alias.go`)
- [x] `dp alias <from> <to>` - Create a mapping
- [x] `dp aliases` - List all aliases
- [x] `dp alias --delete <from>` - Remove an alias
- [x] Aliases stored in SQLite `aliases` table
- [x] Prevent duplicate aliases (upsert behavior)

### 3.4 Init command (`internal/cli/init_cmd.go`)
- [x] `dp init` - Interactive setup wizard
- [x] `dp init --claude-code` - Write Claude Code hook config
  - [x] Detect `~/.claude/settings.json` existence
  - [x] Merge PostToolUseFailure hook into existing config
  - [x] Don't clobber existing hooks
  - [x] Print confirmation with instructions
- [x] Future: `--cursor`, `--copilot` flags for other tools

## Done when

```bash
./dp similar read_file
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
