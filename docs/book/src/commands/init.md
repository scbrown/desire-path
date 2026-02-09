# dp init

Set up integration with AI coding tools

## Usage

    dp init [flags]

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| --source | "" | Source plugin name (required) |
| --track-all | false | Record all invocations, not just failures |
| --claude-code | false | DEPRECATED: Use --source claude-code instead |

## Examples

    $ dp init --source claude-code
    Initialized desire_path integration for claude-code
    Updated: /home/user/.config/claude-code/settings.json

    $ dp init --source claude-code --track-all
    Initialized desire_path integration for claude-code (tracking all invocations)
    Updated: /home/user/.config/claude-code/settings.json

    $ dp init --source cursor
    Initialized desire_path integration for cursor
    Updated: /home/user/.cursor/config.json

## Details

The init command configures hooks in your AI coding tool's settings to automatically capture tool call data. It locates the tool's configuration file, merges in the necessary hooks, and preserves existing settings.

The integration is non-destructive: init will never clobber existing configuration. It merges hooks intelligently, so you can run init multiple times safely.

By default, only failed tool calls are recorded. Use `--track-all` to capture every tool invocation, which is useful for analyzing usage patterns and generating comprehensive statistics.

The `--claude-code` flag is deprecated. Use `--source claude-code` instead for consistency with other commands.

After running init, the AI tool will automatically send tool call data to desire_path. You don't need to manually pipe output or modify your workflow.

If the source plugin doesn't support automatic initialization (no config file to modify), init will print instructions for manual setup.
