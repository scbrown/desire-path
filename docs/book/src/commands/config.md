# dp config

Show or modify configuration

## Usage

    dp config
    dp config <key>
    dp config <key> <value>

## Flags

None

## Examples

    $ dp config
    Configuration (from /home/user/.dp/config.toml):

    db_path: /home/user/.dp/desires.db
    default_source: claude-code
    known_tools: Read,Write,Edit,Bash,Glob,Grep,Task,WebFetch,WebSearch,NotebookEdit
    default_format: json

    $ dp config default_source
    claude-code

    $ dp config default_source cursor
    Updated configuration: default_source = cursor

    $ dp config known_tools "Read,Write,Edit,Bash,CustomTool"
    Updated configuration: known_tools = Read,Write,Edit,Bash,CustomTool

    $ dp config db_path /home/user/projects/myapp/.dp/desires.db
    Updated configuration: db_path = /home/user/projects/myapp/.dp/desires.db

## Details

The config command manages desire_path's persistent configuration. Configuration is stored in `~/.dp/config.toml` and applies to all invocations unless overridden by flags.

Valid configuration keys:

### db_path
Path to the SQLite database file. Default: `~/.dp/desires.db`

Use this to:
- Store project-specific desires in the project directory
- Separate desire data by workspace or client
- Back up or version control desire history

Can be overridden per-command with the global `--db` flag.

### default_source
Default source identifier for commands that accept `--source`. Default: empty

Use this to:
- Avoid typing `--source` repeatedly
- Set a consistent source when working primarily with one AI tool
- Simplify commands: `dp record` instead of `dp record --source claude-code`

Can be overridden per-command with the `--source` flag.

### known_tools
Comma-separated list of known tool names for similarity matching. Default: Read,Write,Edit,Bash,Glob,Grep,Task,WebFetch,WebSearch,NotebookEdit

Use this to:
- Customize the tool set for your environment
- Add custom tools to the suggestion engine
- Match your AI coding tool's specific tool naming conventions

Can be overridden in `dp similar` with the `--known` flag.

### default_format
Default export format: json or csv. Default: json

Use this to:
- Set a preferred output format for exports
- Simplify commands: `dp export` instead of `dp export --format csv`

Can be overridden in `dp export` with the `--format` flag.

Configuration precedence (highest to lowest):
1. Command-line flags (e.g., `--db`, `--source`)
2. Config file values
3. Built-in defaults

To reset a configuration value to its default, delete the key from `~/.dp/config.toml` manually or set it to an empty string:

    $ dp config default_source ""
    Updated configuration: default_source = (unset)

If the config file doesn't exist, it's created automatically on first write. The file is TOML formatted for easy manual editing:

```toml
db_path = "/home/user/.dp/desires.db"
default_source = "claude-code"
known_tools = ["Read", "Write", "Edit", "Bash", "Glob", "Grep", "Task", "WebFetch", "WebSearch", "NotebookEdit"]
default_format = "json"
```
