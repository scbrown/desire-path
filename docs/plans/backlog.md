# Backlog

Ideas validated but deferred. Each entry links to the plan it came from.

---

## Go Template Argument Passthrough for Recipes

**From:** [007 - Recipe Aliases](007-recipe-aliases.md)

When a recipe fires on a prefix match, trailing arguments from the original
command are currently dropped. A future version could capture them and make
them available inside the recipe script via Go `text/template` syntax:

```go
type recipeContext struct {
    Full    string   // full original command string
    Matched string   // the prefix that matched
    Args    []string // tokens after the matched prefix
    Raw     string   // raw text of the matched segment
}

func (r recipeContext) Arg(n int) string { ... } // 1-indexed positional access
```

Usage in a recipe:

```bash
dp alias --recipe "gt convoy wait" \
  'convoy_id={{.Arg 1}}; while true; do
  status=$(gt convoy status "$convoy_id" 2>&1)
  if echo "$status" | grep -qE "complete|failed"; then break; fi
  sleep 10
done'
```

**Why `{{.Arg 1}}` not `$1`:** Shell positional variables (`$1`, `$2`) would
collide with real shell variables in the recipe script (e.g., `$status`). Go
template syntax is syntactically distinct.

**Why deferred:** All current motivating examples (gt await-signal, bd list
--wisp, bd --gated, gt mail check) need no argument passthrough. Adding
templates is additive — can be done when a real use case appears.

**Requires:** Export `Tokenize` from `internal/cmdparse/cmdparse.go` for
parsing trailing args.
