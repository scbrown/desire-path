# Command Reference

The desire_path CLI provides commands for recording, analyzing, and fixing tool call failures in AI coding workflows.

## Command Categories

### Record & Ingest
Commands for capturing tool call data from AI coding tools.

- **record** - Record a failed tool call from stdin
- **ingest** - Ingest tool call data from a source plugin
- **init** - Set up integration with AI coding tools

### Query & Analyze
Commands for exploring recorded desire paths.

- **list** - List recent desires
- **paths** - Show aggregated paths ranked by frequency
- **inspect** - Show detailed view of a specific desire path
- **stats** - Show summary statistics
- **export** - Export raw desire or invocation data

### Map & Fix
Commands for resolving tool name mismatches.

- **similar** - Find known tools similar to a tool name
- **alias** - Create, update, or delete tool name aliases and command correction rules
- **aliases** - List all configured aliases and rules
- **pave** - Turn aliases into active tool-call intercepts

### Configure
Commands for managing configuration.

- **config** - Show or modify configuration

## All Commands

| Command | Description |
|---------|-------------|
| record | Record a failed tool call from stdin |
| ingest | Ingest tool call data from a source plugin |
| init | Set up integration with AI coding tools |
| list | List recent desires |
| paths | Show aggregated paths ranked by frequency |
| inspect | Show detailed view of a specific desire path |
| stats | Show summary statistics |
| export | Export raw desire or invocation data |
| similar | Find known tools similar to a tool name |
| alias | Create, update, or delete tool name aliases and correction rules |
| aliases | List all configured aliases and rules |
| pave | Turn aliases into active tool-call intercepts |
| config | Show or modify configuration |

## Global Flags

All commands support these global flags:

| Flag | Default | Description |
|------|---------|-------------|
| --db PATH | ~/.dp/desires.db | Path to the SQLite database |
| --json | false | Output results as JSON |
