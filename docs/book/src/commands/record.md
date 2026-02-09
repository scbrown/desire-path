# dp record

Record a failed tool call from stdin

## Usage

    dp record [flags]

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| --source | "" | Source identifier for the tool call |

## Examples

    $ echo '{"tool_name":"read_file","error":"unknown tool"}' | dp record --source claude-code
    Recorded desire: read_file

    $ echo '{"tool_name":"file_read","error":"tool not found","input":{"path":"/etc/hosts"}}' | dp record --source cursor
    Recorded desire: file_read

## Details

The record command expects a JSON object from stdin containing at minimum a `tool_name` field. The JSON can include additional fields like `error`, `input`, `timestamp`, and `metadata` which will be stored with the desire.

If no `--source` flag is provided, the source will be recorded as empty. The source helps identify which AI coding tool generated the failed call.

The command reads the entire stdin buffer before parsing, so it works with both piped input and heredocs.

If the JSON is malformed or missing the required `tool_name` field, an error is returned and nothing is recorded.
