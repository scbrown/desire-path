# Desires

A **desire** is a single failed AI tool call. It's the atomic unit of desire_path: one moment where an AI assistant tried to do something and couldn't.

## What Gets Captured

Every desire records:

- **tool_name** (required): The tool the AI tried to call
- **tool_input**: The arguments it tried to pass (JSON)
- **error**: The error message or reason for failure
- **source**: Which AI system generated this (e.g., "claude-code")
- **session_id**: Groups desires from the same conversation
- **cwd**: The working directory when the call failed
- **timestamp**: When it happened (auto-generated)
- **metadata**: Additional context as JSON (optional)

Behind the scenes, each desire also gets a UUID for tracking.

## How Desires Get Recorded

### Manual Recording

Pipe JSON to `dp record`:

```bash
echo '{
  "tool_name": "read_file",
  "tool_input": {"path": "/etc/config.yaml"},
  "error": "tool not found",
  "source": "claude-code"
}' | dp record --source claude-code
```

### Automatic Recording

Set up hooks in your AI tool to automatically capture failures:

```bash
# Initialize desire_path for Claude Code
dp init --source claude-code

# Now failures get recorded automatically as you work
```

When Claude Code tries to call a non-existent tool, the failure flows into desire_path without manual intervention.

## Example Scenario

You're working with Claude Code. It tries to call `read_file` to read a configuration file. But that tool doesn't exist in Claude Code's toolkit (the real tool is `Read`).

The failure gets recorded as a desire:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "tool_name": "read_file",
  "tool_input": {
    "path": "/home/user/config.yaml"
  },
  "error": "tool 'read_file' not found",
  "source": "claude-code",
  "session_id": "session-abc123",
  "cwd": "/home/user/project",
  "timestamp": "2026-02-09T10:30:00Z"
}
```

## Why This Matters

Individual desires are data points. But when you see `read_file` fail 47 times across multiple sessions, that's a **path** — a clear signal that the AI expects this capability. That's when you know to build an adapter, create an alias, or extend your toolset.

Desires are the raw material. Paths are the insight.
