# Plan: Extend Pave with Flag-Aware Command Corrections + Docs

## Context

Pave currently only intercepts wrong **tool names** (`read_file` → block). The user wants to extend it to **correct parameters inside tool calls** — particularly CLI command corrections like `scp -r` → `scp -R`. This needs flag-aware parsing: understanding that `-r` is a flag to `scp`, handling combined flags (`-rP` → `-RP`), and scoping corrections to the right command in pipes.

Claude Code's PreToolUse hook supports this via `updatedInput` (exit 0 + JSON stdout with rewritten parameters).

**Three deliverables:**
1. Flag-aware command correction rules, unified under `dp alias`
2. Updated pave-check hook that rewrites parameters via `updatedInput`
3. Documentation for `dp pave`

**Future extension (not this PR, but data model must support it):** Detect corrections automatically by comparing failed command → next successful command in the same session, then suggest aliases. Desires already store `tool_input`; invocations have it in `metadata`.

---

## CLI Design

Everything through `dp alias`. New flags enter command-correction mode:

```bash
# Tool name alias (unchanged):
dp alias read_file Read

# Flag correction (--cmd + --flag):
dp alias --cmd scp --flag r R
dp alias --cmd scp --flag r R --message "scp uses -R (not -r) for recursive"
dp alias --cmd tar --flag z j --message "Use bzip2 instead of gzip"

# Command substitution (--cmd + --replace):
dp alias --cmd grep --replace rg
dp alias --cmd grep --replace rg --message "Use ripgrep instead of grep"

# Literal string replacement in command context (--cmd + positional args):
dp alias --cmd scp "user@host:" "user@newhost:" --message "Host migrated"

# Advanced / MCP tools (--tool + --param for arbitrary parameter rewrites):
dp alias --tool MyMCPTool --param input_path "/old/path" "/new/path"
dp alias --tool Bash --param command --regex "curl -k" "curl --cacert /etc/ssl/cert.pem"

# Delete (specify same flags to identify the rule):
dp alias --delete --cmd scp --flag r
dp alias --delete --cmd grep --replace grep
dp alias --delete read_file

# List all:
dp aliases
```

**Flag semantics:**
- `--cmd NAME` — enters command mode. Implies tool=Bash, param=command. Scopes to commands where NAME is the program.
- `--flag OLD NEW` — flag correction within the command. Understands `-r`, `--recursive`, combined `-rP`.
- `--replace NEW` — substitute the command itself (grep → rg). Only needs one arg since `--cmd` provides the old name.
- `--tool` + `--param` — advanced: arbitrary tool/parameter targeting (for MCP tools, non-Bash scenarios).
- `--regex` — treat the `from` pattern as a regex (only with `--tool`/`--param`).
- `--message` — custom explanation shown when the correction fires.

**Validation:**
- `--cmd` and `--tool`/`--param` are mutually exclusive
- `--flag` requires `--cmd`
- `--replace` requires `--cmd`
- `--regex` requires `--tool`/`--param`
- `--tool` and `--param` must appear together

---

## Data Model

**File:** `internal/model/model.go`

```go
type Alias struct {
    From      string    `json:"from"`
    To        string    `json:"to"`
    Tool      string    `json:"tool,omitempty"`       // "" = tool-name alias
    Param     string    `json:"param,omitempty"`
    Command   string    `json:"command,omitempty"`     // target CLI command (e.g., "scp")
    MatchKind string    `json:"match_kind,omitempty"`  // "flag", "literal", "command", "regex"
    Message   string    `json:"message,omitempty"`
    CreatedAt time.Time `json:"created_at"`
}
```

**MatchKind values:**
| Value | Meaning | Example |
|-------|---------|---------|
| `""` | Tool name alias | `read_file → Read` |
| `"flag"` | Flag correction | `-r → -R` in scp |
| `"literal"` | String replacement in command | `user@host: → user@newhost:` |
| `"command"` | Command substitution | `grep → rg` |
| `"regex"` | Regex pattern | `curl -k → curl --cacert ...` |

---

## Schema Migration (v2 → v3)

**File:** `internal/store/sqlite.go`

Recreate aliases table (PK changes from single to composite):

```sql
CREATE TABLE aliases_v3 (
    from_name  TEXT NOT NULL,
    to_name    TEXT NOT NULL,
    tool       TEXT NOT NULL DEFAULT '',
    param      TEXT NOT NULL DEFAULT '',
    command    TEXT NOT NULL DEFAULT '',
    match_kind TEXT NOT NULL DEFAULT '',
    message    TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    PRIMARY KEY (from_name, tool, param, command, match_kind)
);

INSERT INTO aliases_v3 (from_name, to_name, tool, param, command, match_kind, message, created_at)
    SELECT from_name, to_name, '', '', '', '', '', created_at FROM aliases;

DROP TABLE aliases;
ALTER TABLE aliases_v3 RENAME TO aliases;
CREATE INDEX idx_aliases_tool_command ON aliases(tool, command);
UPDATE schema_version SET version = 3;
```

---

## Store Interface

**File:** `internal/store/store.go`

```go
SetAlias(ctx context.Context, alias model.Alias) error
GetAlias(ctx context.Context, from, tool, param, command, matchKind string) (*model.Alias, error)
GetAliases(ctx context.Context) ([]model.Alias, error)               // unchanged sig
DeleteAlias(ctx context.Context, from, tool, param, command, matchKind string) (bool, error)
GetRulesForTool(ctx context.Context, tool string) ([]model.Alias, error)  // NEW
```

Also update `GetPaths`/`InspectPath` LEFT JOIN to filter `AND a.tool = '' AND a.param = ''`.

**Implementations to update:** `internal/store/sqlite.go`, `internal/store/remote.go`

**Server routes to update:** `internal/server/server.go` — add `GET /api/v1/aliases/rules?tool=X`, update existing alias handlers for composite key.

---

## Command Parser Package

**New file:** `internal/cmdparse/cmdparse.go`

Small, focused package for shell command manipulation. Used by pave-check and (future) suggestion engine.

```go
package cmdparse

// Segment represents one command in a pipeline or chain.
type Segment struct {
    Command string   // program name (e.g., "scp")
    Args    []string // all arguments as tokens
    Raw     string   // original text of this segment
    Start   int      // byte offset in full command string
    End     int      // byte offset end
}

// Parse splits a command string into Segments on |, &&, ||, ;
func Parse(cmd string) []Segment

// CorrectFlag finds flag `old` in a segment's args (handles combined
// short flags like -rP → -RP) and returns the corrected full command.
// Returns (corrected, true) or ("", false) if flag not found.
func CorrectFlag(segment Segment, oldFlag, newFlag string) (string, bool)

// SubstituteCommand replaces the command name in a segment.
func SubstituteCommand(segment Segment, newCmd string) string

// ReplaceLiteral does a scoped string replacement within a segment.
func ReplaceLiteral(segment Segment, old, new string) string
```

**Flag correction details:**
- Standalone: `-r file` → `-R file`
- Combined: `-rP 22` → `-RP 22` (replace the char within the group)
- Long: `--recursive` → exact match replacement
- Does NOT match `-r` inside filenames or quoted strings

**Parsing approach:**
- Split on unquoted `|`, `&&`, `||`, `;` to get segments
- Respect single/double quotes and backslash escapes
- For each segment: first non-flag token = command name
- Use `go-shellquote` or a simple state machine (no external dep preferred for a hook that runs on every tool call)

**Test file:** `internal/cmdparse/cmdparse_test.go`

---

## Hook Handler

**File:** `internal/cli/pave_check.go`

Expand `hookPayload` to include `tool_input`:

```go
type hookPayload struct {
    ToolName  string                 `json:"tool_name"`
    ToolInput map[string]interface{} `json:"tool_input"`
}
```

Two-phase check:

### Phase 1: Tool-name aliases (unchanged behavior)
`GetAlias(ctx, toolName, "", "", "", "")` → if found, exit 2 + stderr message. Use `alias.Message` if set, else default template.

### Phase 2: Parameter correction rules
`GetRulesForTool(ctx, toolName)` → for each rule, based on `MatchKind`:

- **`"flag"`**: Parse `tool_input[rule.Param]` as shell command, find segment where command = `rule.Command`, apply `CorrectFlag(segment, rule.From, rule.To)`.
- **`"command"`**: Find segment where command = `rule.Command` (which is also `rule.From`), apply `SubstituteCommand(segment, rule.To)`.
- **`"literal"`**: Find segment where command = `rule.Command`, apply `ReplaceLiteral(segment, rule.From, rule.To)`.
- **`"regex"`**: Apply `regexp.Compile(rule.From)` then `ReplaceAllString` on the full parameter value.

If any corrections applied, output:
```json
{
  "hookSpecificOutput": {
    "permissionDecision": "allow",
    "updatedInput": {"command": "corrected value"},
    "additionalContext": "Corrected: scp -r → scp -R (scp uses -R for recursive)"
  }
}
```

Multiple rules can fire on one call. Apply sequentially (compose). Fail-safe: any error → allow unmodified.

---

## Agents-MD Changes

**File:** `internal/cli/pave.go`

Split aliases into tool-name aliases and command rules. Generate grouped output:

```markdown
# Tool Name Corrections
- Do NOT call `read_file`. Use `Read` instead.

# Command Corrections
## scp
- Flag `-r` should be `-R` (scp uses -R for recursive)

## grep → rg
- Use `rg` instead of `grep`
```

---

## Documentation

### New: `docs/book/src/commands/pave.md`
- Overview of pave concept (aliases → active enforcement)
- `--hook` mode: how it works, what pave-check does, exit codes
- `--agents-md` mode: what it generates, --append
- Parameter corrections: how command rules use `updatedInput`
- Flag-aware matching explained with examples
- Troubleshooting (timeout, hook not firing)

### Update: `docs/book/src/SUMMARY.md`
Add `- [dp pave](./commands/pave.md)` after alias entry.

### Update: `docs/book/src/commands/README.md`
Add `pave` to "Map & Fix" section and "All Commands" table.

### Update: `docs/book/src/commands/alias.md`
Add sections for `--cmd`, `--flag`, `--replace`, `--tool`/`--param`, `--regex`, `--message`.

### Update: `docs/book/src/concepts/aliases.md`
Add section on command correction rules as extension of alias concept.

### Update: `docs/book/src/integrations/claude-code.md`
Add section on PreToolUse hook, `updatedInput`, and parameter rewriting.

---

## Implementation Order

1. **`internal/cmdparse/`** — new package with Parse, CorrectFlag, SubstituteCommand, ReplaceLiteral + tests
2. **`internal/model/model.go`** — extend Alias struct
3. **`internal/store/store.go`** — update interface signatures, add GetRulesForTool
4. **`internal/store/sqlite.go`** — migrateV3 + all updated alias methods
5. **`internal/store/remote.go`** — updated methods
6. **`internal/server/server.go`** — updated handlers + rules route
7. **`internal/cli/alias.go`** — new flags (--cmd, --flag, --replace, --tool, --param, --regex, --message)
8. **`internal/cli/pave_check.go`** — parameter rewrite logic using cmdparse
9. **`internal/cli/pave.go`** — updated agents-md output
10. **Tests** — update existing + new for all changed code
11. **Documentation** — pave.md + updates

---

## Verification

```bash
# Unit + integration tests
go test ./...

# Backward compat: tool-name alias still blocks
dp alias read_file Read
echo '{"tool_name":"read_file"}' | dp pave-check  # exit 2

# Flag correction
dp alias --cmd scp --flag r R
echo '{"tool_name":"Bash","tool_input":{"command":"scp -r file.txt host:/"}}' | dp pave-check
# exit 0, stdout JSON: updatedInput.command = "scp -R file.txt host:/"

# Combined flag correction
echo '{"tool_name":"Bash","tool_input":{"command":"scp -rP 22 file host:/"}}' | dp pave-check
# exit 0, command = "scp -RP 22 file host:/"

# Command substitution
dp alias --cmd grep --replace rg
echo '{"tool_name":"Bash","tool_input":{"command":"grep -rn pattern ."}}' | dp pave-check
# exit 0, command = "rg -rn pattern ."

# Pipe scoping
echo '{"tool_name":"Bash","tool_input":{"command":"cat file | grep pattern"}}' | dp pave-check
# only grep is replaced: "cat file | rg pattern"

# List all rules
dp aliases --json

# agents-md includes command rules
dp pave --agents-md

# Docs build
cd docs/book && mdbook build
```

---

## Files Summary

| File | Change |
|------|--------|
| `internal/cmdparse/cmdparse.go` | **NEW** — command parser |
| `internal/cmdparse/cmdparse_test.go` | **NEW** — parser tests |
| `internal/model/model.go` | Add Command, MatchKind, Message fields |
| `internal/store/store.go` | Update interface, add GetRulesForTool |
| `internal/store/sqlite.go` | migrateV3, update all alias methods |
| `internal/store/remote.go` | Update alias methods, add GetRulesForTool |
| `internal/server/server.go` | Add rules route, update alias handlers |
| `internal/cli/alias.go` | New flags, validation, updated set/delete/list |
| `internal/cli/pave_check.go` | Dual-mode: block + rewrite with cmdparse |
| `internal/cli/pave.go` | Updated agents-md with command sections |
| `internal/cli/pave_test.go` | Tests for param rules + agents-md |
| `internal/store/sqlite_test.go` | Migration + new method tests |
| `internal/store/remote_test.go` | Updated routes + tests |
| `internal/integration/pave_check_test.go` | Integration tests for rewrites |
| `docs/book/src/commands/pave.md` | **NEW** — pave command reference |
| `docs/book/src/SUMMARY.md` | Add pave entry |
| `docs/book/src/commands/README.md` | Add pave to tables |
| `docs/book/src/commands/alias.md` | Add command correction docs |
| `docs/book/src/concepts/aliases.md` | Add command correction concept |
| `docs/book/src/integrations/claude-code.md` | Add PreToolUse + rewrite docs |
