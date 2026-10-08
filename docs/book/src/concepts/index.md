# Concepts

The desire_path CLI tracks how AI coding assistants try to interact with your system. Four core concepts form the backbone of `dp`:

## Desires

A **desire** is a single failed tool call from an AI assistant. When Claude Code tries to call `read_file` but that tool doesn't exist, that failure gets recorded as a desire. Each one captures what the AI wanted to do, what went wrong, and the context around it.

[Learn more about desires →](./desires.md)

## Paths

A **path** emerges when the same tool fails repeatedly. Like a worn trail across a lawn, frequent failures for `read_file` form a pattern that says "build a sidewalk here." Paths show you what capabilities to prioritize building.

[Learn more about paths →](./paths-concept.md)

## Aliases

An **alias** maps a hallucinated tool name to a real one. When Claude keeps calling `read_file` but your tool is actually named `Read`, create an alias. This connects desires to reality and helps you understand what the AI is actually trying to accomplish.

[Learn more about aliases →](./aliases.md)

## Invocations

**Invocations** track ALL tool calls, not just failures. When enabled, you get the full picture: success rates, usage patterns, session timelines. This turns desire_path from a failure tracker into a comprehensive telemetry system for AI tool usage.

[Learn more about invocations →](./invocations.md)

---

**The flow**: AI assistants generate desires (failures). Repeated desires form paths (patterns). Aliases connect desires to real tools. Invocations expand tracking to include successes, giving you the complete story.
