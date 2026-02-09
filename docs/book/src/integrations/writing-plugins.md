# Writing a Source Plugin

Source plugins let dp integrate with any AI coding assistant. If you're using a tool that dp doesn't yet support, you can write a plugin in about 50 lines of Go. This guide shows you how.

## Plugin Interface

Every plugin implements `source.Source`:

```go
package source

type Source interface {
    // Name returns the unique identifier for this source (e.g., "my-tool").
    Name() string

    // Extract parses raw bytes and returns universal fields.
    Extract(raw []byte) (*Fields, error)
}
```

The `Extract` method receives raw bytes (usually JSON from a hook or log) and returns structured fields:

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

Only `ToolName` is required. Everything else is optional. Source-specific fields (anything not mapped to the universal fields above) go into `Extra` as raw JSON.

## Minimal Plugin Example

Here's a skeleton plugin for a hypothetical "my-tool":

```go
package source

import (
    "encoding/json"
    "fmt"
)

// myTool implements Source for the "my-tool" AI assistant.
type myTool struct{}

// Register the plugin at startup.
func init() {
    Register(&myTool{})
}

// Name returns the source identifier.
func (m *myTool) Name() string {
    return "my-tool"
}

// Extract parses the my-tool JSON format and returns Fields.
func (m *myTool) Extract(raw []byte) (*Fields, error) {
    // Example input format:
    // {"name": "read_file", "input": {...}, "err": "not found", "session": "abc123", "dir": "/tmp"}

    var payload struct {
        Name    string          `json:"name"`
        Input   json.RawMessage `json:"input"`
        Err     string          `json:"err"`
        Session string          `json:"session"`
        Dir     string          `json:"dir"`
    }

    if err := json.Unmarshal(raw, &payload); err != nil {
        return nil, fmt.Errorf("my-tool: parsing JSON: %w", err)
    }

    if payload.Name == "" {
        return nil, fmt.Errorf("my-tool: missing required field: name")
    }

    fields := &Fields{
        ToolName:   payload.Name,
        InstanceID: payload.Session,
        ToolInput:  payload.Input,
        CWD:        payload.Dir,
        Error:      payload.Err,
    }

    return fields, nil
}
```

Save this as `internal/source/mytool.go`.

## Field Mapping Guidelines

Map your tool's output to universal fields using these conventions:

| Universal Field | Purpose | Examples |
|----------------|---------|----------|
| `ToolName` | The tool/function/command that was invoked | `"Read"`, `"execute_shell"`, `"query_database"` |
| `InstanceID` | Session, request, or invocation ID | Session UUID, request trace ID, user ID |
| `ToolInput` | Raw JSON input parameters | Tool arguments as JSON (preserve as-is) |
| `CWD` | Working directory at time of call | `"/home/user/project"` |
| `Error` | Error message if the call failed | `"File not found"`, `"Permission denied"` |
| `Extra` | Everything else | Anything specific to your tool |

### ToolName (Required)

Must not be empty. This is the key field—dp aggregates desires by tool name. Use the name the AI tried to invoke, even if it doesn't exist.

### InstanceID (Optional)

Ideally a session or request ID that groups related tool calls. Used for:
- Session-level analysis
- Tracing a sequence of calls
- Filtering by session in queries

If your tool doesn't have sessions, use a user ID, request timestamp, or leave it empty.

### ToolInput (Optional)

Preserve the original input as raw JSON. Don't parse or transform it—just copy the bytes. This allows:
- Inspecting common input patterns
- Debugging why a tool failed
- Replaying tool calls

If input is not JSON, wrap it in a JSON string:

```go
fields.ToolInput = json.RawMessage(`"` + rawInput + `"`)
```

### CWD (Optional)

The working directory when the tool was invoked. Useful for:
- Resolving relative paths
- Understanding context
- Project-level aggregation

If your tool doesn't provide this, leave it empty.

### Error (Optional)

Only set this if the tool call failed. For failures, this field should contain a human-readable error message. For successes, leave it empty.

dp uses `Error != ""` to determine if a desire should be recorded.

### Extra (Optional)

Everything not mapped to the universal fields goes here. Examples:
- Internal IDs (like Claude Code's `tool_use_id`)
- Metadata (like `transcript_path`, `permission_mode`)
- Timing information (like `duration_ms`)
- Custom tags or labels

Store as raw JSON:

```go
fields.Extra = map[string]json.RawMessage{
    "tool_id": json.RawMessage(`"xyz123"`),
    "duration_ms": json.RawMessage(`42`),
}
```

## Full Plugin with Extra Fields

Expanding the example:

```go
func (m *myTool) Extract(raw []byte) (*Fields, error) {
    var payload map[string]json.RawMessage
    if err := json.Unmarshal(raw, &payload); err != nil {
        return nil, fmt.Errorf("my-tool: parsing JSON: %w", err)
    }

    var toolName string
    if v, ok := payload["name"]; ok {
        if err := json.Unmarshal(v, &toolName); err != nil {
            return nil, fmt.Errorf("my-tool: parsing name: %w", err)
        }
    }
    if toolName == "" {
        return nil, fmt.Errorf("my-tool: missing required field: name")
    }

    fields := &Fields{ToolName: toolName}

    // Map optional universal fields
    if v, ok := payload["session"]; ok {
        json.Unmarshal(v, &fields.InstanceID)
    }
    if v, ok := payload["input"]; ok {
        fields.ToolInput = v
    }
    if v, ok := payload["dir"]; ok {
        json.Unmarshal(v, &fields.CWD)
    }
    if v, ok := payload["err"]; ok {
        json.Unmarshal(v, &fields.Error)
    }

    // Collect everything else into Extra
    knownFields := map[string]bool{
        "name": true, "session": true, "input": true, "dir": true, "err": true,
    }
    extra := make(map[string]json.RawMessage)
    for k, v := range payload {
        if !knownFields[k] {
            extra[k] = v
        }
    }
    if len(extra) > 0 {
        fields.Extra = extra
    }

    return fields, nil
}
```

This pattern—unmarshal to `map[string]json.RawMessage`, extract known fields, collect unknowns into `Extra`—works for most JSON-based tools.

## Installer Interface (Optional)

If you want to support `dp init --source my-tool`, implement `source.Installer`:

```go
type Installer interface {
    Install(opts InstallOpts) error
}

type InstallOpts struct {
    SettingsPath string // Override settings file location (empty = use default)
    TrackAll     bool   // Install hooks for all invocations (not just failures)
}
```

Example:

```go
func (m *myTool) Install(opts InstallOpts) error {
    settingsPath := opts.SettingsPath
    if settingsPath == "" {
        home, err := os.UserHomeDir()
        if err != nil {
            return fmt.Errorf("determine home directory: %w", err)
        }
        settingsPath = filepath.Join(home, ".mytool", "config.json")
    }

    // Read existing config
    data, err := os.ReadFile(settingsPath)
    if os.IsNotExist(err) {
        data = []byte("{}")
    } else if err != nil {
        return fmt.Errorf("read config: %w", err)
    }

    var config map[string]interface{}
    if err := json.Unmarshal(data, &config); err != nil {
        return fmt.Errorf("parse config: %w", err)
    }

    // Add hook configuration
    // (Details depend on your tool's hook system)
    config["hooks"] = map[string]interface{}{
        "on_failure": "dp record --source my-tool",
    }

    if opts.TrackAll {
        config["hooks"].(map[string]interface{})["on_call"] = "dp ingest --source my-tool"
    }

    // Write config back
    newData, err := json.MarshalIndent(config, "", "  ")
    if err != nil {
        return fmt.Errorf("marshal config: %w", err)
    }

    if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil {
        return fmt.Errorf("create config directory: %w", err)
    }

    if err := os.WriteFile(settingsPath, newData, 0o644); err != nil {
        return fmt.Errorf("write config: %w", err)
    }

    return nil
}
```

Make sure the implementation is **idempotent**—running `dp init --source my-tool` twice shouldn't break anything or add duplicate hooks.

## Registering the Plugin

In your plugin file's `init()` function:

```go
func init() {
    Register(&myTool{})
}
```

Then import the plugin package for side effects in `cmd/dp/main.go`:

```go
package main

import (
    _ "github.com/scbrown/desire-path/internal/source" // registers claude-code
    _ "github.com/scbrown/desire-path/internal/source/mytool" // register your plugin
)
```

If your plugin lives in the same package as `claudecode.go` (i.e., `internal/source/mytool.go`), you don't need a separate import—the package `init()` runs automatically.

## Testing

Write tests in `internal/source/mytool_test.go`:

```go
package source

import (
    "encoding/json"
    "testing"
)

func TestMyToolExtract(t *testing.T) {
    plugin := &myTool{}

    input := `{"name":"read_file","input":{"path":"/tmp/test.txt"},"err":"not found","session":"abc","dir":"/home/user"}`

    fields, err := plugin.Extract([]byte(input))
    if err != nil {
        t.Fatalf("Extract failed: %v", err)
    }

    if fields.ToolName != "read_file" {
        t.Errorf("ToolName = %q, want %q", fields.ToolName, "read_file")
    }
    if fields.InstanceID != "abc" {
        t.Errorf("InstanceID = %q, want %q", fields.InstanceID, "abc")
    }
    if fields.Error != "not found" {
        t.Errorf("Error = %q, want %q", fields.Error, "not found")
    }
    if fields.CWD != "/home/user" {
        t.Errorf("CWD = %q, want %q", fields.CWD, "/home/user")
    }

    var input map[string]interface{}
    if err := json.Unmarshal(fields.ToolInput, &input); err != nil {
        t.Fatalf("ToolInput not valid JSON: %v", err)
    }
    if input["path"] != "/tmp/test.txt" {
        t.Errorf("ToolInput.path = %v, want %q", input["path"], "/tmp/test.txt")
    }
}

func TestMyToolMissingName(t *testing.T) {
    plugin := &myTool{}

    input := `{"input":{},"err":"error"}`

    _, err := plugin.Extract([]byte(input))
    if err == nil {
        t.Fatal("expected error for missing name, got nil")
    }
}
```

Run tests:

```bash
go test ./internal/source/...
```

## Integration Testing

Test the full pipeline:

```bash
# Build dp with your plugin
go install ./cmd/dp

# Test extraction via dp record
echo '{"name":"test_tool","err":"test error","session":"manual"}' | dp record --source my-tool

# Verify it was recorded
dp list --limit 1
```

## Real-World Example: Claude Code Plugin

See `internal/source/claudecode.go` for a complete, production-quality plugin. Key features:

- Strict JSON validation with helpful error messages
- Mapping Claude-specific fields (`session_id`, `tool_use_id`) to universal + Extra
- Idempotent `Install()` implementation with hook merging
- Comprehensive tests

Use it as a reference when building your own plugin.

## Plugin Checklist

Before shipping your plugin:

- [ ] Implements `source.Source` interface
- [ ] `Name()` returns a unique, kebab-case identifier
- [ ] `Extract()` validates `ToolName` is non-empty
- [ ] `Extract()` returns helpful error messages
- [ ] Maps universal fields correctly
- [ ] Stores extra fields in `Extra`
- [ ] Registers via `source.Register()` in `init()`
- [ ] Imported in `cmd/dp/main.go`
- [ ] Has tests covering success and error cases
- [ ] Documented in `docs/book/src/integrations/<plugin-name>.md`
- [ ] (Optional) Implements `source.Installer` for `dp init` support
- [ ] (Optional) `Install()` is idempotent

## Distribution

If your plugin is for a public tool, consider submitting it upstream via a pull request to the desire-path repo. If it's for an internal/proprietary tool, maintain it in a separate module and import it:

```go
import (
    _ "github.com/yourorg/desire-path-my-tool-plugin"
)
```

Plugins don't need to live in the main dp repository—they just need to call `source.Register()` at startup.

## Next Steps

- Study the [Claude Code plugin](./claude-code.md) for a real example
- Read the [Architecture](../architecture.md) to understand dp's data flow
- Check out the [source package godoc](https://pkg.go.dev/github.com/scbrown/desire-path/internal/source) for API details
