# Integrations

dp uses a **source plugin system** to integrate with different AI coding assistants. Each AI tool has its own output format, hook mechanism, and session model. Source plugins abstract these differences behind a common interface, allowing dp to record desires and invocations from any tool.

## How It Works

1. **Hook Installation**: The AI tool (like Claude Code) provides event hooks that trigger on tool calls or failures. dp installs shell commands as hook handlers.
2. **Payload Extraction**: When the hook fires, it passes a JSON payload to dp. The source plugin parses this payload and extracts universal fields.
3. **Normalization**: The plugin maps tool-specific fields (like Claude Code's `session_id`) to universal fields (like `instance_id`).
4. **Storage**: dp writes the normalized data to its SQLite database.

## Plugin Architecture

Every source plugin implements the `source.Source` interface:

```go
type Source interface {
    Name() string
    Extract(raw []byte) (*Fields, error)
}
```

The `Extract` method receives raw bytes (usually JSON) and returns structured `Fields`:

```go
type Fields struct {
    ToolName   string          // Required: the tool that was invoked
    InstanceID string          // Optional: session or invocation ID
    ToolInput  json.RawMessage // Optional: raw JSON input to the tool
    CWD        string          // Optional: working directory
    Error      string          // Optional: error message (for failures)
    Extra      map[string]json.RawMessage // Source-specific fields
}
```

Plugins can optionally implement `source.Installer` to support `dp init`:

```go
type Installer interface {
    Install(opts InstallOpts) error
}
```

This allows `dp init --source <name>` to automatically configure hooks in the AI tool's settings.

## Currently Supported Tools

### Claude Code

Status: **Fully supported**

Claude Code provides `PostToolUseFailure` and `PostToolUse` hooks. dp uses these to capture failed tool calls (for desires) or all tool calls (for invocations).

See the [Claude Code Integration Guide](./claude-code.md) for setup instructions and details.

## Planned Integrations

The following tools are planned but not yet implemented:

- **Cursor**: Cursor AI editor (pending hook API documentation)
- **Gemini CLI**: Google's AI CLI (pending output format spec)
- **GitHub Copilot CLI**: `gh copilot` command output parsing
- **Cody**: Sourcegraph's Cody assistant

## Writing Your Own Plugin

If you're using an AI tool that dp doesn't yet support, you can write a plugin. It's just a Go file that implements `source.Source` and calls `source.Register` in `init()`.

See [Writing a Source Plugin](./writing-plugins.md) for a complete guide with examples.

## Plugin Registry

All plugins self-register at startup via `init()` functions. dp discovers plugins by importing them:

```go
import (
    _ "github.com/scbrown/desire-path/internal/source" // registers claude-code
    // Add more plugin imports here
)
```

List available plugins:

```bash
dp init --list
```

This shows all registered source names that can be used with `--source`.

## Hook Execution Model

dp hooks are designed to be:

- **Asynchronous**: The AI tool doesn't block waiting for dp to finish
- **Isolated**: dp failures don't affect the AI tool's operation
- **Lightweight**: Writes are fast; database is append-only with WAL mode

Typical hook latency: <10ms for desire recording, <20ms for invocation ingestion.

## Next Steps

- [Claude Code Integration](./claude-code.md): Detailed guide for Claude Code users
- [Writing a Plugin](./writing-plugins.md): Build your own source plugin
- [Architecture](../architecture.md): Deep dive into dp's internals
