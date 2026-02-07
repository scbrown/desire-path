# Gemini Instructions

Read [AGENTS.md](./AGENTS.md) first - it covers all shared project conventions.

This file covers Gemini-specific guidance for working on this project.

## Key Reminders

- This is a Go project using `cmd/` + `internal/` layout. See AGENTS.md for full conventions.
- Tests use stdlib `testing` only - no third-party test frameworks.
- Keep dependencies minimal. Check AGENTS.md dependency table before adding anything.
- Document new functionality as it's built - update AGENTS.md and relevant docs/ files.

## Build & Verify

```bash
go build -o dp ./cmd/dp       # Build
go test ./...                  # Test all packages
go vet ./...                   # Static analysis
```
