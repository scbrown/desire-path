# dp alias

Create, update, or delete tool name aliases

## Usage

    dp alias <from> <to>
    dp alias --delete <from>
    dp aliases

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| --delete | false | Delete an existing alias |

## Examples

    $ dp alias read_file Read
    Created alias: read_file -> Read

    $ dp alias file_read Read
    Created alias: file_read -> Read

    $ dp alias read_file ReadFile
    Updated alias: read_file -> ReadFile (was: Read)

    $ dp alias --delete read_file
    Deleted alias: read_file

    $ dp aliases
    FROM            TO           CREATED
    read_file       Read         2026-02-01 09:15:33
    file_read       Read         2026-02-01 09:16:12
    grep_search     Grep         2026-02-01 09:17:44
    search_grep     Grep         2026-02-01 09:18:09
    edit_document   Edit         2026-02-01 09:19:22
    write_file      Write        2026-02-01 09:20:15
    bash_exec       Bash         2026-02-01 09:21:04
    run_command     Bash         2026-02-01 09:21:55

    8 aliases configured

## Details

The alias command manages tool name mappings, allowing you to define how incorrect tool names should be resolved to known tools.

Aliases are upserted: creating an alias that already exists will update it to the new target. This makes it safe to run alias commands idempotently.

Multiple incorrect tool names can map to the same known tool. For example, both `read_file` and `file_read` can alias to `Read`.

When you create an alias, desire_path uses it immediately in subsequent commands like `dp suggest` and `dp paths`. Existing desire records are not modified, but queries will show the alias mapping.

The `--delete` flag removes an alias permanently. This is useful when cleaning up obsolete mappings or correcting mistakes.

The `dp aliases` command (plural) lists all configured aliases. It shows:
- FROM: The incorrect tool name that appears in failure records
- TO: The known tool name it maps to
- CREATED: When the alias was created

Common workflow for creating aliases:

1. Run `dp paths` to see the most frequent tool name patterns
2. For each pattern, run `dp suggest <pattern>` to find the best match
3. Create aliases for the top patterns: `dp alias <pattern> <match>`
4. Verify with `dp aliases`

Aliases are stored in the database alongside desire records. They're tied to your database file (default: ~/.dp/desires.db), so different projects can have different alias configurations if they use different databases via `--db`.

Best practices:
- Map to the canonical tool name used by your AI coding tool (e.g., "Read" for Claude Code)
- Be consistent: if you map `read_file` to `Read`, also map `file_read` to `Read`, not `read`
- Document your alias strategy if working in a team
- Review aliases periodically with `dp aliases` to remove obsolete mappings

Aliases make desire_path's analysis commands more useful by normalizing tool names across different failure patterns.
