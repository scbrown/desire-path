# dp ingest

Ingest tool call data from a source plugin

## Usage

    dp ingest [flags]

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| --source | "" | Source plugin name (required) |

## Examples

    $ cat payload.json | dp ingest --source claude-code
    Ingested 3 desires from claude-code

    $ curl https://api.example.com/tool-calls | dp ingest --source custom-plugin
    Ingested 12 desires from custom-plugin

    $ dp ingest --source nonexistent < data.json
    Error: source plugin "nonexistent" not found
    Available sources: claude-code, cursor

## Details

The ingest command reads raw data from stdin and uses a source plugin to parse it into desire records. Unlike `dp record`, which expects pre-formatted JSON, `dp ingest` delegates parsing to a plugin that understands the source's native format.

The `--source` flag is required. If omitted, the command will error and list available source plugins.

Source plugins are responsible for:
- Parsing the input format
- Extracting tool names, errors, inputs, and metadata
- Handling batches of tool calls

This command is useful for bulk imports, integrating with custom AI tools, or processing historical data that wasn't captured in real-time.

Plugins are discovered from the `source/` package. To add a new source, implement the Source interface and register it in the plugin registry.
