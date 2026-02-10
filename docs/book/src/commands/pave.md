# dp pave

Turn aliases and correction rules into active tool-call intercepts.

## Usage

    dp pave --hook
    dp pave --agents-md
    dp pave --agents-md --append AGENTS.md

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| --hook | false | Install a PreToolUse intercept hook in Claude Code |
| --agents-md | false | Generate AGENTS.md / CLAUDE.md rules from alias data |
| --append FILE | | Append generated rules to FILE (with --agents-md) |
| --settings PATH | ~/.claude/settings.json | Path to Claude Code settings file |

## Modes

### --hook: Real-Time Intercept

Installs a `PreToolUse` hook into `~/.claude/settings.json` that runs `dp pave-check` on every tool call. The hook has two behaviors:

**Phase 1 — Tool Name Blocking:** If the AI calls a tool that has a tool-name alias (e.g., `read_file` when the real tool is `Read`), the hook blocks the call with exit code 2 and tells Claude to use the correct name.

**Phase 2 — Parameter Correction:** If the tool name is valid but the parameters contain known mistakes (e.g., `scp -r` instead of `scp -R`), the hook rewrites the parameters automatically via `updatedInput` and allows the call to proceed with corrected values.

```bash
dp pave --hook
```

Output:
```
PreToolUse intercept hook installed!
Hallucinated tool names matching aliases will now be blocked automatically.
Manage aliases with: dp alias <from> <to>
```

Running again is safe — it detects the existing hook and reports "already installed."

### --agents-md: Static Rules

Generates markdown rules from your aliases and correction rules. Output has two sections:

**Tool Name Corrections** — tells the AI which tool names are wrong:

```markdown
# Tool Name Corrections

The following tool names are INCORRECT. Use the correct names instead:

- Do NOT call `read_file`. Use `Read` instead.
- Do NOT call `search_files`. Use `Grep` instead.
```

**Command Corrections** — documents parameter correction rules:

```markdown
# Command Corrections

## scp

- Flag `-r` should be `-R` (scp uses -R for recursive)

## grep → rg

- Use `rg` instead of `grep`
```

By default, output goes to stdout. Use `--append` to write to a file:

```bash
dp pave --agents-md --append AGENTS.md
```

### Belt and Suspenders

Use both modes together for maximum coverage:

```bash
dp pave --hook --agents-md --append AGENTS.md
```

- `--hook` is **reactive**: catches mistakes at call time
- `--agents-md` is **preventive**: stops mistakes before they happen

## How pave-check Works

The `dp pave-check` command is an internal hook handler. It reads a JSON payload from stdin and performs two phases:

### Phase 1: Tool Name Check

Looks up the `tool_name` in the alias table. If found, blocks the call:

- **Exit code 2** + error message on stderr
- Claude Code shows the message and retries with the correct tool name

### Phase 2: Parameter Corrections

Queries correction rules for the tool name via `GetRulesForTool`. For each matching rule:

| MatchKind | What It Does |
|-----------|-------------|
| `flag` | Corrects a CLI flag within a specific command (e.g., `-r` → `-R` in `scp`) |
| `command` | Substitutes a command name (e.g., `grep` → `rg`) |
| `literal` | Replaces a literal string within a command segment |
| `regex` | Applies a regex replacement across the full parameter value |

If corrections are applied:
- **Exit code 0** + JSON on stdout with `updatedInput`
- Claude Code uses the corrected parameters transparently

If no corrections match:
- **Exit code 0** with no output (allow as-is)

### Flag-Aware Matching

The `flag` match kind uses a shell-aware command parser (`cmdparse`) that:

- Splits commands on `|`, `&&`, `||`, `;` to isolate segments
- Respects quoted strings (won't match flags inside quotes)
- Handles combined short flags: `-rP 22` → `-RP 22`
- Scopes corrections to the right command in a pipeline

Example: Given a rule `--cmd scp --flag r R`:

```
scp -r file.txt host:/          →  scp -R file.txt host:/
scp -rP 22 file host:/          →  scp -RP 22 file host:/
cat file | scp -r host:/        →  cat file | scp -R host:/
echo "-r" | scp file host:/     →  (no change — "-r" is in quotes, not a flag)
```

### Pipe Scoping

Command substitutions only affect the matching segment. Given `--cmd grep --replace rg`:

```
cat file | grep pattern | wc -l  →  cat file | rg pattern | wc -l
```

Only `grep` is replaced; `cat` and `wc` are untouched.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Allow (optionally with `updatedInput` corrections) |
| 2 | Block (tool name alias matched) |

## Hook Timeout

The pave-check hook has a 3-second timeout. It typically completes in <50ms. If the database is locked or unreachable, the hook fails open (allows the call).

## Troubleshooting

### Hook Not Firing

Verify installation:

```bash
cat ~/.claude/settings.json | jq '.hooks.PreToolUse'
```

Check that `dp` is in your PATH:

```bash
which dp
```

### Corrections Not Applying

List your rules to verify they exist:

```bash
dp aliases --json
```

Test manually:

```bash
echo '{"tool_name":"Bash","tool_input":{"command":"scp -r file host:/"}}' | dp pave-check
```

You should see JSON output with `updatedInput` if a matching rule exists.

### Hook Timing Out

The default timeout is 3000ms. If your database is on a slow disk:

1. Check database size: `ls -lh ~/.dp/desires.db`
2. Consider running `VACUUM` if the database has grown large
3. Increase the timeout in `~/.claude/settings.json` if needed

## Examples

```bash
# Install the hook
dp pave --hook

# Generate rules to stdout
dp pave --agents-md

# Append rules to CLAUDE.md
dp pave --agents-md --append CLAUDE.md

# Both at once
dp pave --hook --agents-md --append AGENTS.md

# JSON output
dp pave --agents-md --json
```
