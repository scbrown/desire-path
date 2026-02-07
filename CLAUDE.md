# Claude Code Instructions

Read [AGENTS.md](./AGENTS.md) first - it covers all shared project conventions.

This file covers Claude Code-specific guidance for working on this project.

## Tool Preferences

- Use `Read` to read files, not `cat`/`head`/`tail`
- Use `Edit` to modify files, not `sed`/`awk`
- Use `Write` to create files, not `echo` redirection
- Use `Glob` to find files, not `find`/`ls`
- Use `Grep` to search content, not `grep`/`rg`
- Reserve `Bash` for git, go build/test, and actual shell operations

## Testing Workflow

```bash
go test ./...                 # Run all tests
go test ./internal/store/     # Run specific package tests
go test -v -run TestName ./internal/store/  # Run specific test
```

## Hook Integration Testing

This project integrates with Claude Code via `PostToolUseFailure` hooks. To test:

1. Build: `go build -o dp ./cmd/dp`
2. Add to Claude Code settings (`~/.claude/settings.json`):
```json
{
  "hooks": {
    "PostToolUseFailure": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/dp record --source claude-code",
            "async": true
          }
        ]
      }
    ]
  }
}
```
3. Trigger a tool failure in Claude Code, then check: `./dp list`

## Commit Style

- Short imperative subject line (e.g., "add record command", "fix sqlite migration")
- No scope prefixes, no conventional commits unless asked
