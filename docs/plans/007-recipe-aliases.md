# 007 - Recipe/Script Aliases for Multi-Step Command Expansion

## Context

The alias system currently supports five correction modes:

| Mode | What it does | Example |
|------|-------------|---------|
| Tool-name | Block hallucinated tool, suggest correct one | `read_file → Read` |
| Flag | Replace a flag within a command | `scp -r → scp -R` |
| Command | Substitute a command name | `grep → rg` |
| Literal | String replacement in a command segment | `user@host: → user@new:` |
| Regex | Pattern-based replacement on full param value | `curl -k → curl --cacert ...` |

All of these perform **surgical edits** — they modify a piece of the command
while preserving context. But some hallucinated commands can't be fixed with
surgical edits. They need **wholesale replacement** with a different command
or multi-step script:

- `gt await-signal` → polling loop on `gt mol status`
- `bd list --wisp` → `bd list` with type filter (feature gap in bd)
- `bd --gated` → `bd list` with gated filter (feature gap in bd)
- `gt convoy wait` → polling loop on convoy status
- `gt mail check` → `gt mail inbox` (command doesn't exist, but the intent is clear)

These all share a pattern: the agent's intent is correct but the command
doesn't exist (or the flag doesn't exist). A simple rewrite isn't enough —
the correct implementation is a fundamentally different command or a
multi-line script.

**Recipes bridge the gap** between simple rewrites and missing features. They
let us implement shims at the alias layer without waiting for upstream tools
to add the feature.

---

## Design Principles

1. **Recipes are just aliases with longer `to` values.** No new tables, no
   new interfaces. The `match_kind = "recipe"` case fits into the existing
   composite key model.

2. **Prefix matching with word boundaries.** A recipe for `gt await-signal`
   fires on `gt await-signal` and `gt await-signal --verbose`, but NOT on
   `gt await-signaling`. The match requires the next character after the
   prefix to be whitespace or end-of-string.

3. **Full segment replacement.** The entire matched command segment is
   replaced by the recipe script. Trailing arguments are dropped. (Argument
   passthrough via templates is deferred — see [backlog](backlog.md).)

4. **Multi-line is fine.** SQLite TEXT columns, JSON encoding, and Claude
   Code's Bash tool all handle multi-line strings natively.

5. **Fail-safe.** Any error in recipe matching → allow the call unmodified
   (same as all other pave-check error paths).

---

## CLI Interface

New `--recipe` boolean flag on `dp alias`. Same positional arg pattern as a
plain tool-name alias (`dp alias <from> <to>`), just with `--recipe` to
select recipe matching and replacement behavior:

```bash
# Simple recipe: command → replacement script
dp alias --recipe "gt await-signal" 'while true; do
  status=$(gt mol status 2>&1)
  echo "$status"
  if echo "$status" | grep -q "signaled"; then break; fi
  sleep 5
done'

# Recipe with message
dp alias --recipe "bd list --wisp" 'bd list | grep -i wisp' \
  --message "bd has no --wisp flag; filter output instead"

# Simple command remapping
dp alias --recipe "gt mail check" 'gt mail inbox'

# Delete a recipe
dp alias --delete --recipe "gt await-signal"

# List all (recipes appear alongside other aliases)
dp aliases
```

**Validation:**

- `--recipe` is mutually exclusive with `--cmd`, `--tool`, `--param`,
  `--flag`, `--replace`, `--regex`.
- Requires 2 positional args (FROM and TO), or 1 with `--delete`.
- `--message` works as usual.

---

## Data Model

No changes to the `Alias` struct. Recipes use existing fields:

```go
model.Alias{
    From:      "gt await-signal",        // prefix to match (word-boundary aware)
    To:        "while true; do ...",      // replacement script (may be multi-line)
    Tool:      "Bash",                   // always Bash for recipes
    Param:     "command",                // always command for recipes
    Command:   "gt",                     // auto-extracted from From (first token)
    MatchKind: "recipe",                 // new match kind
    Message:   "Poll gt mol status...",  // optional
}
```

**Composite key:** `(from_name, tool, param, command, match_kind)` — recipes
are unique by their `from` pattern.

**Command field:** Auto-extracted from `From` (first token). Enables
`GetRulesForTool` to filter efficiently — when pave-check processes a Bash
call containing `gt ...`, only rules with `command = "gt"` are candidates.

---

## Schema

No migration needed. The `match_kind` column is free TEXT — adding `"recipe"`
requires no DDL changes. The existing `aliases_v3` schema handles it as-is.

---

## Matching Semantics

Recipe matching uses **word-boundary prefix matching** on the parsed command
segment:

```go
func applyRecipeRule(value string, rule model.Alias) (string, string, bool) {
    segs := cmdparse.Parse(value)
    for _, seg := range segs {
        if seg.Command != rule.Command {
            continue
        }
        // Prefix match with word boundary: the segment must start with
        // rule.From and the next character must be whitespace or EOF.
        if !matchRecipePrefix(seg.Raw, rule.From) {
            continue
        }

        // Replace the entire segment with the recipe script.
        full := cmdparse.ApplyToFull(value, seg, rule.To)
        desc := fmt.Sprintf("%s → [recipe]", rule.From)
        if rule.Message != "" {
            desc = rule.Message
        }
        return full, desc, true
    }
    return "", "", false
}

// matchRecipePrefix returns true if s starts with prefix and the character
// immediately after prefix is whitespace or s is exactly prefix.
func matchRecipePrefix(s, prefix string) bool {
    if !strings.HasPrefix(s, prefix) {
        return false
    }
    if len(s) == len(prefix) {
        return true // exact match
    }
    next := s[len(prefix)]
    return next == ' ' || next == '\t'
}
```

**Why prefix, not exact:** Agents add arguments the recipe creator didn't
anticipate. `gt await-signal --verbose` should still trigger the
`gt await-signal` recipe. Trailing args are silently dropped (argument
passthrough is a [backlog item](backlog.md)).

**Why word boundaries:** Without them, a recipe for `bd list --wisp` would
falsely match `bd list --wispy`. The word boundary check (next char is
whitespace or EOF) prevents this.

---

## pave-check Integration

Recipe rules fire in Phase 2 (parameter corrections), alongside existing
rule types. One new case in the `applyRule` switch:

```go
case "recipe":
    return applyRecipeRule(value, rule)
```

Output follows the existing `updatedInput` protocol:

```json
{
  "hookSpecificOutput": {
    "permissionDecision": "allow",
    "updatedInput": {
      "command": "while true; do\n  status=$(gt mol status 2>&1)\n  ..."
    },
    "additionalContext": "Corrected: Poll gt mol status in a loop"
  }
}
```

Claude Code executes the rewritten command automatically.

---

## agents-md Generation

Recipes use the message (or a default) — NOT a truncated script preview.
Showing script fragments in agents-md doesn't help the LLM avoid the
hallucination. What helps is a clear instruction:

```go
case "recipe":
    if r.Message != "" {
        desc = fmt.Sprintf("Do NOT use `%s`. %s", r.From, r.Message)
    } else {
        desc = fmt.Sprintf("Do NOT use `%s` — it does not exist and will be rewritten automatically.", r.From)
    }
```

In grouped output:

```markdown
# Command Corrections

## gt

- Do NOT use `gt await-signal`. Poll gt mol status in a loop
- Do NOT use `gt convoy wait` — it does not exist and will be rewritten automatically.

## bd

- Do NOT use `bd list --wisp`. bd has no --wisp flag; filter output instead
```

---

## aliases List Display

Recipes appear in `dp aliases` with type `recipe`. Long `To` values are
truncated and newlines collapsed for table display:

```
FROM                TO                              TYPE     COMMAND   CREATED
gt await-signal     while true; do status=$(gt...   recipe   gt        2026-02-10
bd list --wisp      bd list | grep -i wisp          recipe   bd        2026-02-10
read_file           Read                            alias              2026-02-10
```

---

## Quoting & Escaping

**Creation:** Single-quote the script to prevent shell expansion:

```bash
dp alias --recipe "gt await-signal" 'while true; do
  status=$(gt mol status 2>&1)
  ...
done'
```

For complex scripts: `dp alias --recipe "gt await-signal" "$(cat recipe.sh)"`

**Storage:** SQLite TEXT handles multi-line and embedded quotes natively.

**Output:** `json.NewEncoder` handles newlines (`\n`) and quotes (`\"`)
automatically.

---

## Implementation Order

1. **`internal/cli/alias.go`** — add `--recipe` flag, recipe mode in
   `buildAlias`, `extractCommand` helper, truncation in `listAliases`.

2. **`internal/cli/pave_check.go`** — add `applyRecipeRule`,
   `matchRecipePrefix`, and `"recipe"` case in `applyRule` switch.

3. **`internal/cli/pave.go`** — add `"recipe"` case in
   `formatRuleDescription`.

4. **Tests:**
   - `alias_test.go`: creation, deletion, validation, mutual exclusivity
   - `pave_check_test.go`: simple match, prefix with trailing args, word
     boundary rejection, pipeline scoping, no-match passthrough
   - `pave_test.go`: agents-md with recipes

---

## Verification

```bash
go test ./...

# Create a recipe
dp alias --recipe "gt await-signal" 'while true; do
  status=$(gt mol status 2>&1)
  echo "$status"
  if echo "$status" | grep -q "signaled"; then break; fi
  sleep 5
done' --message "Poll gt mol status in a loop"

# List
dp aliases

# pave-check fires
echo '{"tool_name":"Bash","tool_input":{"command":"gt await-signal"}}' \
  | dp pave-check
# exit 0, stdout: updatedInput.command = polling loop

# Prefix match with trailing args (args dropped, recipe fires)
echo '{"tool_name":"Bash","tool_input":{"command":"gt await-signal --verbose"}}' \
  | dp pave-check
# exit 0, recipe fires

# Word boundary: "gt await-signaling" does NOT match
echo '{"tool_name":"Bash","tool_input":{"command":"gt await-signaling"}}' \
  | dp pave-check
# exit 0, no correction (passthrough)

# Pipeline scoping: only matched segment replaced
echo '{"tool_name":"Bash","tool_input":{"command":"echo start && gt await-signal"}}' \
  | dp pave-check
# exit 0, "echo start" preserved, gt segment replaced

# Delete
dp alias --delete --recipe "gt await-signal"

# Backward compat
dp alias --cmd scp --flag r R
echo '{"tool_name":"Bash","tool_input":{"command":"scp -r file host:/"}}' \
  | dp pave-check
# exit 0, flag correction still works
```

---

## Files Summary

| File | Change |
|------|--------|
| `internal/cli/alias.go` | `--recipe` flag, recipe mode, `extractCommand`, list truncation |
| `internal/cli/pave_check.go` | `applyRecipeRule`, `matchRecipePrefix`, recipe case in switch |
| `internal/cli/pave.go` | Recipe case in `formatRuleDescription` |
| `internal/cli/alias_test.go` | Recipe creation/deletion/validation tests |
| `internal/cli/pave_check_test.go` | Recipe matching + pave-check tests |
| `internal/cli/pave_test.go` | agents-md with recipes test |

No changes to cmdparse, model, store, schema, or server.

---

## Design Decisions

**Why a new match_kind, not extending regex:** Regex rules apply
`ReplaceAllString` — surgical edit in place. Recipes replace the entire
segment wholesale. The matching semantics (prefix vs pattern) and replacement
semantics (wholesale vs surgical) are different enough to warrant a distinct
kind.

**Why prefix matching, not exact:** Agents add arguments the recipe creator
didn't anticipate. Prefix matching with word boundaries is the pragmatic
default.

**Why no argument passthrough in v1:** All motivating examples need no args.
Templates add complexity (Go text/template import, context type, error
handling). Deferred to [backlog](backlog.md) until a real use case appears.

**Why not a separate `recipes` table:** Recipes share the alias lifecycle
(create, list, delete), hook mechanism (pave-check), and output mechanism
(agents-md). The composite key model handles uniqueness.

**Why Bash-only:** All motivating examples are Bash commands. Tool-name
recipes can be added later if there's demand.

---

## Future Extensions

See [backlog](backlog.md) for deferred items including:
- Go template argument passthrough
- Tool-name recipes (exit 2 with script in message)
- Recipe validation (`--dry-run`)
- Recipe from file (`--file recipe.sh`)
