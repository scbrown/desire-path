# 007 - Recipe/Script Aliases for Multi-Step Command Expansion

## Context

The alias system currently supports four correction modes:

| Mode | What it does | Example |
|------|-------------|---------|
| Tool-name | Block hallucinated tool, suggest correct one | `read_file → Read` |
| Flag | Replace a flag within a command | `scp -r → scp -R` |
| Command | Substitute a command name | `grep → rg` |
| Literal | String replacement in a command segment | `user@host: → user@new:` |
| Regex | Pattern-based replacement on full param value | `curl -k → curl --cacert ...` |

All of these perform **surgical edits** — they modify a piece of the command while preserving context. But some hallucinated commands can't be fixed with surgical edits. They need **wholesale replacement** with a multi-step script:

- `gt await-signal` → polling loop on `gt mol status`
- `bd list --wisp` → `bd list` with type filter (feature gap in bd)
- `bd --gated` → `bd list` with gated filter (feature gap in bd)
- `gt convoy wait` → polling loop on convoy status
- `gt mail check` → `gt mail inbox` (command doesn't exist, but the intent is clear)

These all share a pattern: the agent's intent is correct but the command doesn't exist (or the flag doesn't exist). A simple rewrite isn't enough — the correct implementation is a fundamentally different command or a multi-line script.

**Recipes bridge the gap** between simple rewrites and missing features. They let us implement shims at the alias layer without waiting for upstream tools to add the feature.

---

## Design Principles

1. **Recipes are just aliases with longer `to` values.** No new tables, no new interfaces. The `match_kind = "recipe"` case fits into the existing composite key model.

2. **Prefix matching by default.** A recipe for `gt await-signal` should fire whether the agent typed `gt await-signal` or `gt await-signal --timeout 30`. The recipe replaces the entire matched segment.

3. **Template variables for argument passthrough.** When the agent passes arguments that the recipe should preserve, template variables (`{{.Args}}`) let the recipe script reference them.

4. **Multi-line is fine.** The `to` field in SQLite TEXT columns handles multi-line strings. JSON encoding handles them for `updatedInput`. Claude Code's Bash tool handles multi-line command parameters.

5. **Fail-safe.** If a recipe's template expansion fails, allow the call unmodified (same as all other pave-check error paths).

---

## CLI Interface

New `--recipe` flag on `dp alias`:

```bash
# Simple recipe: exact command → replacement
dp alias --recipe "gt await-signal" 'while true; do
  status=$(gt mol status 2>&1)
  echo "$status"
  if echo "$status" | grep -q "signaled"; then break; fi
  sleep 5
done'

# Recipe with message
dp alias --recipe "bd list --wisp" 'bd list | grep -i wisp' \
  --message "bd has no --wisp flag; filter output instead"

# Recipe with argument passthrough
dp alias --recipe "gt convoy wait" \
  'convoy_id={{.Arg 1}}; while true; do
  status=$(gt convoy status "$convoy_id" 2>&1)
  echo "$status"
  if echo "$status" | grep -qE "complete|failed"; then break; fi
  sleep 10
done'

# Delete a recipe
dp alias --delete --recipe "gt await-signal"

# List all (recipes appear alongside other aliases)
dp aliases
```

**Flag semantics:**

- `--recipe FROM TO` — recipe mode. Implies `tool=Bash`, `param=command`, `match_kind=recipe`.
- The `FROM` is the command prefix to match (first positional arg).
- The `TO` is the replacement script (second positional arg).
- `--message` works as usual.
- `--delete --recipe FROM` removes the recipe.

**Validation:**

- `--recipe` is mutually exclusive with `--cmd`, `--tool`, `--param`, `--flag`, `--replace`, `--regex`.
- `--recipe` requires exactly 2 positional args (FROM and TO), or 1 with `--delete`.

---

## Data Model

No changes to the `Alias` struct. Recipes use existing fields:

```go
model.Alias{
    From:      "gt await-signal",        // prefix to match
    To:        "while true; do ...",      // replacement script (may be multi-line)
    Tool:      "Bash",                   // always Bash for recipes
    Param:     "command",                // always command for recipes
    Command:   "gt",                     // extracted from From (first token)
    MatchKind: "recipe",                 // new match kind
    Message:   "Poll gt mol status...",  // optional
}
```

**Composite key:** `(from_name, tool, param, command, match_kind)` — recipes are unique by their `from` pattern. Two recipes can't have the same `from` value (which is correct — you can't have two different expansions for the same command).

**Command field:** Extracted automatically from `From` — it's the first token (program name). This enables `GetRulesForTool` to filter efficiently: when pave-check processes a Bash call containing `gt ...`, only rules with `command = "gt"` are candidates.

---

## Schema

No migration needed. The `match_kind` column is a free TEXT field — adding `"recipe"` requires no DDL changes. The existing `aliases_v3` schema handles it:

```sql
-- Already exists:
CREATE TABLE aliases_v3 (
    from_name  TEXT NOT NULL,
    to_name    TEXT NOT NULL,   -- stores the full script, multi-line OK
    tool       TEXT NOT NULL DEFAULT '',
    param      TEXT NOT NULL DEFAULT '',
    command    TEXT NOT NULL DEFAULT '',
    match_kind TEXT NOT NULL DEFAULT '',
    message    TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    PRIMARY KEY (from_name, tool, param, command, match_kind)
);
```

---

## Template Variables

Recipes support Go `text/template` syntax for argument passthrough. The template context provides:

```go
type recipeContext struct {
    Full    string   // full original command string
    Matched string   // the segment that matched
    Args    []string // tokens after the matched prefix
    Raw     string   // raw text of the matched segment
}

// Helper method for positional access
func (r recipeContext) Arg(n int) string {
    if n < 1 || n > len(r.Args) {
        return ""
    }
    return r.Args[n-1]
}
```

**Examples:**

| Recipe From | Agent Types | Args | Template Usage |
|-------------|------------|------|---------------|
| `gt await-signal` | `gt await-signal` | `[]` | No template needed |
| `gt convoy wait` | `gt convoy wait abc123` | `["abc123"]` | `{{.Arg 1}}` |
| `bd list --wisp` | `bd list --wisp --limit 10` | `["--limit", "10"]` | `{{.Args}}` (joined) |

**Template expansion happens in pave-check**, not at alias creation time. If the template fails to expand (bad syntax, etc.), the recipe is skipped (fail-safe).

**Escape hatch:** If a recipe doesn't use `{{...}}` at all, no template processing occurs — the `to` value is used as-is. This means simple recipes (the common case) have zero template overhead.

---

## Matching Semantics

Recipe matching in pave-check uses **prefix matching on the segment's raw text**:

```go
func applyRecipeRule(value string, rule model.Alias) (string, string, bool) {
    segs := cmdparse.Parse(value)
    for _, seg := range segs {
        if seg.Command != rule.Command {
            continue
        }
        // Prefix match: does this segment start with the recipe's From pattern?
        if !strings.HasPrefix(seg.Raw, rule.From) {
            continue
        }
        // Extract trailing args (everything after the matched prefix).
        trailing := strings.TrimSpace(seg.Raw[len(rule.From):])
        var args []string
        if trailing != "" {
            args = cmdparse.Tokenize(trailing) // need to export tokenize
        }

        // Expand template if needed.
        expanded, err := expandRecipe(rule.To, recipeContext{
            Full:    value,
            Matched: rule.From,
            Args:    args,
            Raw:     seg.Raw,
        })
        if err != nil {
            return "", "", false // template error → skip
        }

        full := cmdparse.ApplyToFull(value, seg, expanded)
        desc := fmt.Sprintf("%s → [recipe]", rule.From)
        if rule.Message != "" {
            desc = rule.Message
        }
        return full, desc, true
    }
    return "", "", false
}
```

**Why prefix match, not exact match:**

Agents frequently add arguments the recipe creator didn't anticipate. `gt await-signal --timeout 30` should still trigger the `gt await-signal` recipe. The trailing args are captured and available via template variables.

**Why not regex:**

Regex matching already exists as `match_kind = "regex"`. Recipes solve a different problem (wholesale replacement, not pattern-based substitution). Keeping prefix matching simple and predictable avoids the cognitive overhead of debugging regex + template interactions.

---

## pave-check Integration

Recipe rules fire in Phase 2 (parameter corrections), alongside existing rule types:

```go
// In applyRule(), add the recipe case:
func applyRule(value string, rule model.Alias) (string, string, bool) {
    switch rule.MatchKind {
    case "flag":
        return applyFlagRule(value, rule)
    case "command":
        return applyCommandRule(value, rule)
    case "literal":
        return applyLiteralRule(value, rule)
    case "regex":
        return applyRegexRule(value, rule)
    case "recipe":
        return applyRecipeRule(value, rule)
    default:
        return "", "", false
    }
}
```

Output follows the existing `updatedInput` protocol:

```json
{
  "hookSpecificOutput": {
    "permissionDecision": "allow",
    "updatedInput": {
      "command": "while true; do\n  status=$(gt mol status 2>&1)\n  echo \"$status\"\n  if echo \"$status\" | grep -q \"signaled\"; then break; fi\n  sleep 5\ndone"
    },
    "additionalContext": "Corrected: gt await-signal → [recipe] (Poll gt mol status in a loop)"
  }
}
```

Claude Code will execute the rewritten command automatically — the agent sees the multi-line script as the command it "intended" to run.

---

## agents-md Generation

Recipes get their own description format in `formatRuleDescription`:

```go
case "recipe":
    // Show the from pattern and a truncated preview of the script.
    preview := r.To
    if len(preview) > 60 {
        preview = preview[:57] + "..."
    }
    // Replace newlines with spaces for inline display.
    preview = strings.ReplaceAll(preview, "\n", " ")
    desc = fmt.Sprintf("Do NOT use `%s`. It will be expanded to: `%s`", r.From, preview)
```

In the grouped output:

```markdown
# Command Corrections

## gt (recipes)

- Do NOT use `gt await-signal`. It will be expanded to: `while true; do status=$(gt mol status 2>&1) echo "$status" ...` (Poll gt mol status in a loop)
- Do NOT use `gt convoy wait`. It will be expanded to: `convoy_id=$1; while true; do status=$(gt convoy status ...`
```

---

## CLI Changes

**File:** `internal/cli/alias.go`

Add `aliasRecipe bool` flag and Mode 6 in `buildAlias`:

```go
var aliasRecipe bool

func init() {
    aliasCmd.Flags().BoolVar(&aliasRecipe, "recipe", false,
        "recipe mode: replace entire command with a script")
    // ... existing flags
}
```

**New mode in buildAlias (after existing modes, before Mode 5):**

```go
// Mode 6: --recipe (full command replacement)
if aliasRecipe {
    // Validate mutual exclusivity.
    if aliasCmd_ != "" || aliasTool != "" || aliasParam != "" ||
        len(aliasFlag) > 0 || aliasReplace != "" || aliasRegex {
        return a, fmt.Errorf("--recipe cannot be combined with --cmd, --tool, --param, --flag, --replace, or --regex")
    }

    if aliasDelete {
        if len(args) != 1 {
            return a, fmt.Errorf("--delete --recipe requires one argument: the FROM pattern")
        }
        a.From = args[0]
        a.Tool = "Bash"
        a.Param = "command"
        a.Command = extractCommand(args[0]) // first token
        a.MatchKind = "recipe"
        return a, nil
    }

    if len(args) != 2 {
        return a, fmt.Errorf("--recipe requires two arguments: FROM SCRIPT")
    }
    a.From = args[0]
    a.To = args[1]
    a.Tool = "Bash"
    a.Param = "command"
    a.Command = extractCommand(args[0])
    a.MatchKind = "recipe"
    return a, nil
}
```

`extractCommand` is a small helper:

```go
func extractCommand(from string) string {
    tokens := strings.Fields(from)
    if len(tokens) == 0 {
        return ""
    }
    return tokens[0]
}
```

---

## Quoting & Escaping

**Creation:** The recipe script is the second positional argument. Shell quoting rules apply at the CLI level — users wrap the script in single quotes to prevent shell expansion:

```bash
dp alias --recipe "gt await-signal" 'while true; do
  status=$(gt mol status 2>&1)
  ...
done'
```

Alternatively, for complex scripts, pipe from a file:

```bash
dp alias --recipe "gt await-signal" "$(cat recipe.sh)"
```

**Storage:** SQLite TEXT columns handle multi-line strings and embedded quotes natively. No special escaping needed.

**Output (pave-check):** JSON encoding via `json.NewEncoder` handles newlines (→ `\n`) and quotes (→ `\"`) automatically. Claude Code's Bash tool accepts multi-line command strings.

**Template variables:** Go's `text/template` handles escaping within template expansion. Shell-special characters in `{{.Arg 1}}` are preserved literally — the recipe author is responsible for quoting them properly in the template (e.g., `"{{.Arg 1}}"` to handle spaces in args).

---

## cmdparse Changes

Export the `tokenize` function (currently lowercase) so recipe matching can parse trailing args:

```go
// Tokenize splits a command string into tokens, respecting quotes and escapes.
// Exported for use by recipe argument extraction.
func Tokenize(s string) []string {
    return tokenize(s)
}
```

This is the only change to cmdparse. The existing `Parse`, `ApplyToFull`, and segment model work as-is.

---

## aliases List Display

Recipes appear in `dp aliases` with type `recipe`:

```
FROM                TO                              TYPE     COMMAND   CREATED
gt await-signal     while true; do status=$(gt...   recipe   gt        2026-02-10 19:00:00
bd list --wisp      bd list | grep -i wisp          recipe   bd        2026-02-10 19:00:00
read_file           Read                            alias              2026-02-10 18:00:00
```

The `To` column should be truncated for long scripts. Update `listAliases` to truncate:

```go
to := a.To
if len(to) > 40 {
    to = to[:37] + "..."
}
// Also replace newlines for table display.
to = strings.ReplaceAll(to, "\n", " ")
```

---

## Implementation Order

1. **`internal/cmdparse/cmdparse.go`** — export `Tokenize` (rename `tokenize` → `Tokenize`, add `tokenize` as alias for backward compat, or just capitalize).

2. **`internal/cli/alias.go`** — add `--recipe` flag, Mode 6 in `buildAlias`, `extractCommand` helper, truncation in `listAliases`.

3. **`internal/cli/pave_check.go`** — add `applyRecipeRule` function, `recipeContext` type, `expandRecipe` helper, and `"recipe"` case in `applyRule` switch.

4. **`internal/cli/pave.go`** — add `"recipe"` case in `formatRuleDescription`, update group headers for recipe rules.

5. **Tests** — unit tests for each change:
   - `alias_test.go`: TestAliasCmdRecipe (creation, deletion, validation)
   - `pave_check_test.go`: TestRecipeRuleSimple, TestRecipeRuleWithArgs, TestRecipeRuleTemplateError, TestRecipeRulePipeline
   - `pave_test.go`: TestAgentsMDWithRecipes
   - `cmdparse_test.go`: TestTokenizeExported

6. **Integration tests** (in `internal/integration/`):
   - TestRecipeEndToEnd: create recipe → pipe hook payload → verify updatedInput contains expanded script
   - TestRecipeWithArguments: verify template variable expansion

---

## Verification

```bash
# Unit + integration tests
go test ./...

# Create a recipe
dp alias --recipe "gt await-signal" 'while true; do
  status=$(gt mol status 2>&1)
  echo "$status"
  if echo "$status" | grep -q "signaled"; then break; fi
  sleep 5
done' --message "Poll gt mol status in a loop"

# Verify it appears in list
dp aliases

# Test pave-check fires the recipe
echo '{"tool_name":"Bash","tool_input":{"command":"gt await-signal"}}' | dp pave-check
# exit 0, stdout JSON: updatedInput.command contains the polling loop script

# Test with trailing args
echo '{"tool_name":"Bash","tool_input":{"command":"gt await-signal --verbose"}}' | dp pave-check
# exit 0, prefix match fires, trailing args available via template

# Test agents-md includes recipe
dp pave --agents-md
# Output includes recipe description

# Test pipe scoping — recipe only matches its segment
echo '{"tool_name":"Bash","tool_input":{"command":"echo start && gt await-signal"}}' | dp pave-check
# exit 0, only the gt segment is replaced

# Delete the recipe
dp alias --delete --recipe "gt await-signal"

# Verify backward compat: existing rules still work
dp alias --cmd scp --flag r R
echo '{"tool_name":"Bash","tool_input":{"command":"scp -r file host:/"}}' | dp pave-check
# exit 0, flag correction still works
```

---

## Files Summary

| File | Change |
|------|--------|
| `internal/cmdparse/cmdparse.go` | Export `Tokenize` |
| `internal/cli/alias.go` | Add `--recipe` flag, Mode 6, `extractCommand`, truncation in list |
| `internal/cli/pave_check.go` | Add `applyRecipeRule`, `recipeContext`, `expandRecipe`, recipe case |
| `internal/cli/pave.go` | Add recipe case in `formatRuleDescription` and group headers |
| `internal/cli/alias_test.go` | Recipe creation/deletion/validation tests |
| `internal/cli/pave_check_test.go` | Recipe rule application tests |
| `internal/cli/pave_test.go` | agents-md with recipes test |
| `internal/cmdparse/cmdparse_test.go` | Tokenize export test |
| `internal/integration/pave_check_test.go` | Recipe end-to-end integration tests |

---

## Design Decisions

**Why a new match_kind, not extending regex:** Regex rules apply `ReplaceAllString` — they edit in place. Recipes replace the *entire segment*. The matching semantics (prefix vs pattern) and replacement semantics (wholesale vs surgical) are different enough to warrant a distinct kind. Overloading regex would make the behavior confusing ("sometimes regex edits, sometimes it replaces entirely").

**Why prefix matching, not exact matching:** Agents hallucinate commands but often add reasonable arguments. `gt await-signal --timeout 30` should still trigger the recipe for `gt await-signal`. Exact matching would require users to anticipate every possible argument combination. Prefix matching is the pragmatic default.

**Why templates, not positional shell variables:** `$1`, `$2` etc. would conflict with actual shell variables in the recipe script (e.g., a recipe that uses `$status`). Go templates with `{{.Arg 1}}` are syntactically distinct and won't collide.

**Why not a separate `recipes` table:** Recipes are aliases — they share the same lifecycle (create, list, delete), the same hook mechanism (pave-check), and the same output mechanism (agents-md). A separate table would duplicate all of this infrastructure. The composite key model already handles the uniqueness constraint.

**Why Bash-only for v1:** All the motivating examples are Bash commands. Tool-name recipes (hallucinated tool → "run this script instead") would use exit 2 with the recipe in the message, which is just guidance — the agent has to manually run it. Command recipes via updatedInput are automatic. We can add tool-name recipes later if there's demand.

**Why no stdin-from-file for recipe body:** YAGNI for v1. Shell quoting handles multi-line strings fine (`'...'`), and `"$(cat file)"` works as an escape hatch. Adding `--recipe-file` can come later if users find the shell quoting painful.

---

## Future Extensions (Not This PR)

- **Tool-name recipes:** `dp alias --recipe-tool await_signal '...'` — when agent calls hallucinated tool `await_signal`, block with exit 2 and include the recipe script in the message. The agent would then manually use Bash to run it.

- **Recipe validation:** `dp alias --recipe --dry-run` to test template expansion without saving.

- **Recipe from file:** `dp alias --recipe "gt await-signal" --file recipe.sh` for complex scripts.

- **Auto-recipe suggestion:** When `dp suggest` can't find a simple alias, suggest recipe creation based on common patterns in the desire log.
