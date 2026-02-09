# Configuration

dp stores configuration in `~/.dp/config.toml`. You can view and update settings using the `dp config` command or by editing the file directly.

## Config File Location

By default: `~/.dp/config.toml`

Override with the `DESIRE_PATH_CONFIG` environment variable:

```bash
export DESIRE_PATH_CONFIG=/path/to/custom/config.toml
```

## Valid Configuration Keys

| Key | Type | Description | Default |
|-----|------|-------------|---------|
| `db_path` | string | Path to the SQLite database file | `~/.dp/desires.db` |
| `default_source` | string | Default source tag for recorded desires when `--source` is not specified | `""` (empty) |
| `known_tools` | string | Comma-separated list of known tool names used by `dp suggest` | `""` (empty—uses built-in list) |
| `default_format` | string | Default output format: `"table"` or `"json"` | `"table"` |

## Usage Examples

### View Current Config

```bash
# Show all settings
dp config

# Show a specific key
dp config db_path
```

### Set Values

```bash
# Change database path
dp config db_path /data/dp/desires.db

# Set default source
dp config default_source claude-code

# Use JSON output by default
dp config default_format json
```

### Configure Known Tools

The `known_tools` setting controls the list of tool names used by `dp suggest` for similarity matching. If empty, dp uses a built-in default list (currently: `Read`, `Write`, `Edit`, `Bash`, `Grep`, `Glob`, `WebSearch`, `WebFetch`).

Set a custom list:

```bash
dp config known_tools "Read,Write,Edit,Bash,CustomTool,AnotherTool"
```

dp splits on commas and trims whitespace, so you can use spaces for readability:

```bash
dp config known_tools "Read, Write, Edit, Bash, CustomTool"
```

### Reset to Default

To clear a setting and revert to the default, set it to an empty string:

```bash
dp config known_tools ""
```

## Global Flags

Some configuration can be overridden per-command using global flags:

### Database Path

```bash
# Use a different database for one command
dp --db /tmp/test.db list

# Set via environment variable
export DESIRE_PATH_DB=/tmp/test.db
dp list
```

Precedence: `--db` flag > `DESIRE_PATH_DB` env var > `db_path` config > default (`~/.dp/desires.db`)

### Output Format

```bash
# Force JSON output
dp --json paths

# Force table output (default)
dp paths
```

The `--json` flag overrides `default_format` config for that command.

## Advanced: Direct File Editing

`~/.dp/config.toml` is plain TOML. Example:

```toml
db_path = "/data/dp/desires.db"
default_source = "claude-code"
known_tools = ["Read", "Write", "Edit", "Bash", "CustomTool"]
default_format = "json"
```

If the file doesn't exist, dp creates it on first write. Invalid TOML causes an error — use `dp config` for safer editing.

> **Migration note:** Legacy `config.json` files are automatically migrated to TOML on first load.

## Database Configuration

### Database Path

The SQLite database stores desires, invocations, aliases, and schema metadata. By default it lives at `~/.dp/desires.db`.

Change the path permanently:

```bash
dp config db_path /new/path/desires.db
```

Or temporarily:

```bash
dp --db /tmp/test.db list
```

### SQLite Options

dp uses pure-Go SQLite (via `modernc.org/sqlite`) with WAL mode enabled for concurrent reads and writes. There are no user-configurable SQLite options—the database is tuned for dp's access patterns.

If you need to inspect the database directly:

```bash
sqlite3 ~/.dp/desires.db
```

Tables: `desires`, `invocations`, `aliases`, `schema_version`.

## Source Plugin Configuration

Each source plugin (like `claude-code`) may have its own configuration needs. Check the integration docs:

- [Claude Code Integration](./integrations/claude-code.md)
- [Writing a Plugin](./integrations/writing-plugins.md)

For Claude Code specifically, the hook configuration lives in `~/.claude/settings.json`, not in dp's config file. Run `dp init --source claude-code` to set it up.

## Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `DESIRE_PATH_DB` | Override database path | `export DESIRE_PATH_DB=/tmp/test.db` |
| `DESIRE_PATH_CONFIG` | Override config file path | `export DESIRE_PATH_CONFIG=/etc/dp/config.toml` |

Environment variables take precedence over config file settings but are overridden by command-line flags.

## Tips

- Use `--json` output with `jq` for scripting: `dp paths --json | jq '.[] | select(.count > 10)'`
- Keep `known_tools` in sync with your AI's actual tool set for better `suggest` results
- If you're testing or developing, point `--db` at a temporary database to avoid polluting your real data
- The config file is optional—all keys have sensible defaults

For command-specific options, see the [Command Reference](./commands/README.md).
