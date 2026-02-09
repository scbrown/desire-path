# dp export

Export raw desire or invocation data

## Usage

    dp export [flags]

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| --format | json | Output format: json or csv |
| --since | "" | Filter by RFC3339 timestamp or YYYY-MM-DD |
| --type | desires | Data type to export: desires or invocations |

## Examples

    $ dp export --format json
    {"id":1,"tool_name":"read_file","error":"unknown tool","source":"claude-code","timestamp":"2026-02-09T14:32:15Z","input":"{\"path\":\"/etc/hosts\"}"}
    {"id":2,"tool_name":"file_read","error":"tool not found","source":"claude-code","timestamp":"2026-02-09T14:31:42Z","input":"{\"file_path\":\"/home/user/config.json\"}"}
    {"id":3,"tool_name":"edit_document","error":"invalid parameters","source":"cursor","timestamp":"2026-02-09T14:28:33Z","input":"{}"}

    $ dp export --format csv --since 2026-02-01
    id,tool_name,error,source,timestamp,input,metadata
    89,read_file,unknown tool,claude-code,2026-02-09T14:32:15Z,"{\"path\":\"/etc/hosts\"}",
    90,file_read,tool not found,claude-code,2026-02-09T14:31:42Z,"{\"file_path\":\"/home/user/config.json\"}",
    91,edit_document,invalid parameters,cursor,2026-02-09T14:28:33Z,"{}",

    $ dp export --format json --since 2026-02-08T00:00:00Z | jq -r '.tool_name' | sort | uniq -c | sort -rn
         24 read_file
         18 file_read
         12 grep_search
          9 edit_document
          7 write_file

    $ dp export --format json --type invocations --since 2026-02-09
    {"id":1,"tool_name":"Read","success":true,"source":"claude-code","timestamp":"2026-02-09T14:35:22Z","duration_ms":45,"input":"{\"file_path\":\"/etc/hosts\"}"}
    {"id":2,"tool_name":"read_file","success":false,"source":"claude-code","timestamp":"2026-02-09T14:32:15Z","duration_ms":12,"error":"unknown tool"}
    {"id":3,"tool_name":"Bash","success":true,"source":"claude-code","timestamp":"2026-02-09T14:30:08Z","duration_ms":234,"input":"{\"command\":\"ls -la\"}"}

## Details

The export command outputs raw data for external processing, analysis, or archival. It's designed for piping to other tools or importing into data analysis platforms.

JSON format outputs JSONL (JSON Lines): one complete JSON object per line. This format is easy to process with `jq`, `jless`, or stream into other systems:

    dp export --format json | jq 'select(.source == "claude-code")'

CSV format includes headers and is suitable for importing into spreadsheets or databases. Fields containing commas or quotes are properly escaped.

Use `--since` to export data from a specific date forward. Accepts RFC3339 timestamps (`2026-02-09T00:00:00Z`) or simple dates (`2026-02-09`).

Use `--type` to choose between exporting desire (failure) data or invocation (all tool call) data. Invocation data is only available if you've enabled tracking with `dp init --track-all`.

Common export workflows:

Analyze tool name patterns:

    dp export --format json | jq -r '.tool_name' | sort | uniq -c | sort -rn

Find all errors from a specific source:

    dp export --format json | jq 'select(.source == "claude-code") | .error' | sort | uniq -c

Export to CSV for spreadsheet analysis:

    dp export --format csv > desires.csv

Backup your data:

    dp export --format json > backup-$(date +%Y%m%d).jsonl

The export command never modifies the database. It's read-only and safe to run at any time.
