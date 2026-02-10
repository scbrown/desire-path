# Aliases

An **alias** maps a hallucinated tool name to a real one. When the AI keeps calling `read_file` but your actual tool is named `Read`, an alias bridges that gap.

## Why Aliases Exist

AI assistants often hallucinate tool names that feel natural but don't match your actual API. Claude Code might try:

- `read_file` when the real tool is `Read`
- `execute_command` when it's `Bash`
- `search_files` when it's `Glob`

These aren't bugs in the AI — they're reasonable guesses. But they create friction. Aliases let you connect desires to reality without renaming your tools or retraining models.

## Creating an Alias

Basic syntax:

```bash
dp alias <hallucinated_name> <real_tool_name>
```

Example:

```bash
dp alias read_file Read
```

Now when you run `dp paths` or `dp suggest`, desires for `read_file` show their connection to `Read`.

## Upsert Behavior

Creating an alias twice updates the target:

```bash
dp alias read_file Read      # Maps read_file → Read
dp alias read_file ReadFile  # Updates to read_file → ReadFile
```

No error, no duplicate entries. The latest mapping wins.

## Listing Aliases

See all current mappings:

```bash
dp aliases
```

Output:

```
ALIAS            MAPS TO
read_file        Read
execute_bash     Bash
search_files     Glob
```

## Deleting an Alias

Remove a mapping when it's no longer needed:

```bash
dp alias --delete read_file
```

The desires remain in your database, but the mapping is gone. `dp paths` will show the raw pattern again.

## How Aliases Appear in Commands

### In `dp paths`

Without alias:

```
PATTERN         COUNT   FIRST SEEN            LAST SEEN
read_file       47      2026-02-01 09:15:23   2026-02-09 10:30:45
```

With alias:

```
PATTERN         COUNT   FIRST SEEN            LAST SEEN             ALIAS
read_file       47      2026-02-01 09:15:23   2026-02-09 10:30:45   Read
```

### In `dp suggest`

Suggestions incorporate alias information to show what the AI is trying to accomplish with tools that actually exist.

## When To Use Aliases

### Perfect Match, Wrong Name

The AI's concept matches your tool exactly, just different naming:
- `read_file` → `Read`
- `execute_bash` → `Bash`

### Subset Mapping

The AI wants something specific that's part of a broader tool:
- `search_codebase` → `Grep`
- `list_directory` → `Glob`

### Don't Alias If...

- The concepts don't actually align (forcing a bad mapping creates confusion)
- You're planning to build the hallucinated tool anyway (let the path data guide development)

## Command Correction Rules

Aliases extend beyond tool names. **Command correction rules** fix mistakes *inside* tool parameters — like wrong CLI flags, deprecated command names, or outdated paths.

### The Problem

AI assistants don't just hallucinate tool names. They also misremember CLI flags:

- `scp -r` when the correct flag is `scp -R`
- `grep pattern` when you prefer `rg pattern`
- `curl -k` when you need `curl --cacert cert.pem`

These are the same kind of desire path: the AI's intent is correct, but the specifics are wrong.

### Rule Types

| Type | What It Fixes | Example |
|------|--------------|---------|
| Flag | Wrong CLI flag | `-r` → `-R` in `scp` |
| Command | Wrong command name | `grep` → `rg` |
| Literal | Wrong string in a command | `user@old:` → `user@new:` |
| Regex | Pattern-based replacement | `curl -k` → `curl --cacert cert.pem` |

### How They Work

Rules are scoped to a specific command within a pipeline. Given a Bash tool call like:

```
cat file.txt | scp -rP 22 user@host:/path
```

A flag rule for `scp` with `-r` → `-R` would:

1. Parse the pipeline into segments: `cat file.txt` and `scp -rP 22 user@host:/path`
2. Find the segment where the command is `scp`
3. Locate the `-r` flag within `-rP` (combined flags)
4. Rewrite to `-RP`: `cat file.txt | scp -RP 22 user@host:/path`

The correction is precise: it only touches the right command and the right flag.

### Creating Rules

```bash
# Flag correction
dp alias --cmd scp --flag r R --message "scp uses -R for recursive"

# Command substitution
dp alias --cmd grep --replace rg

# Literal replacement
dp alias --cmd scp "user@old:" "user@new:"
```

### Enforcement

Rules are enforced by `dp pave --hook`, which installs a PreToolUse hook in Claude Code. When a tool call matches a rule, the hook rewrites the parameters before the tool executes. The AI sees a note about the correction in the response.

See [dp pave](../commands/pave.md) for details on the hook mechanism.

## Aliases as Documentation

Your alias list is a rosetta stone between AI expectations and your actual API. It documents the gap between "what AI assistants naturally try to do" and "what your system actually provides."

That gap is valuable data. Don't just fix it — learn from it.
