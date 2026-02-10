# dp alias

Create, update, or delete tool name aliases and command correction rules.

## Usage

    dp alias <from> <to>
    dp alias --delete <from>
    dp alias --cmd <name> --flag <old> <new>
    dp alias --cmd <name> --replace <new>
    dp alias --cmd <name> <from> <to>
    dp alias --tool <tool> --param <param> <from> <to>
    dp aliases

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| --delete | false | Delete an existing alias or rule |
| --cmd NAME | | Command name for CLI corrections (implies tool=Bash, param=command) |
| --flag OLD,NEW | | Flag correction within a command (requires --cmd) |
| --replace NEW | | Substitute the command itself (requires --cmd) |
| --tool NAME | | Tool name for parameter corrections (advanced) |
| --param NAME | | Parameter name to correct (requires --tool) |
| --regex | false | Treat FROM as a regex pattern (requires --tool/--param) |
| --message TEXT | | Custom message shown when correction fires |

## Tool Name Aliases

Map a hallucinated tool name to the correct one:

```bash
dp alias read_file Read
dp alias search_files Grep
dp alias --delete read_file
```

## Command Flag Corrections

Fix incorrect CLI flags scoped to a specific command:

```bash
# scp uses -R (not -r) for recursive
dp alias --cmd scp --flag r R

# With a custom message
dp alias --cmd scp --flag r R --message "scp uses -R for recursive"

# Delete the rule
dp alias --delete --cmd scp --flag r
```

When `dp pave --hook` is active, this automatically rewrites `scp -r` to `scp -R` in any Bash tool call. Combined flags are handled too: `-rP 22` becomes `-RP 22`.

## Command Substitution

Replace one command with another:

```bash
# Use ripgrep instead of grep
dp alias --cmd grep --replace rg

# With a message
dp alias --cmd grep --replace rg --message "Use ripgrep instead of grep"
```

This rewrites `grep -rn pattern .` to `rg -rn pattern .` while leaving other commands in a pipeline untouched.

## Literal Replacement

Replace a literal string within a specific command's context:

```bash
dp alias --cmd scp "user@old-host:" "user@new-host:" --message "Host migrated"
```

## Advanced: Tool/Param Corrections

For non-Bash tools or arbitrary parameter corrections:

```bash
# Correct a path in an MCP tool
dp alias --tool MyMCPTool --param input_path "/old/path" "/new/path"

# Regex replacement
dp alias --tool Bash --param command --regex "curl -k" "curl --cacert cert.pem"
```

## Listing Rules

```bash
dp aliases
```

Output:

```
FROM            TO           TYPE      COMMAND   CREATED
read_file       Read         alias               2026-02-01 09:15:33
r               R            flag      scp       2026-02-01 09:16:12
grep            rg           command   grep      2026-02-01 09:17:44
```

## Validation

- `--cmd` and `--tool`/`--param` are mutually exclusive
- `--flag` requires `--cmd`
- `--replace` requires `--cmd`
- `--flag` and `--replace` are mutually exclusive
- `--regex` requires `--tool`/`--param`
- `--tool` and `--param` must appear together

## Details

The alias command manages both tool name mappings and command correction rules. Tool name aliases define how incorrect tool names should be resolved. Command correction rules define how parameters should be rewritten when a tool is called.

Aliases and rules are upserted: creating one that already exists updates it. This makes it safe to run commands idempotently.

Rules are identified by a composite key: `(from, tool, param, command, match_kind)`. This means you can have multiple rules for the same command targeting different flags.

When you create rules, they take effect immediately if `dp pave --hook` is installed. The hook checks rules on every tool call and applies corrections transparently.

Common workflow for command corrections:

1. Notice the AI keeps using wrong flags (e.g., `scp -r` fails)
2. Create a rule: `dp alias --cmd scp --flag r R`
3. Install the hook: `dp pave --hook`
4. Future `scp -r` calls are automatically corrected to `scp -R`

Best practices:
- Add `--message` to explain *why* the correction exists
- Use `--cmd` for CLI corrections (most common case)
- Use `--tool`/`--param` only for MCP or non-Bash tools
- Review rules with `dp aliases` periodically
