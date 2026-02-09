# Introduction

Picture a university campus. The architects laid careful concrete sidewalks connecting every building. But students cut across the grass, wearing paths between the library and the dorms, from the quad to the parking lot. Those worn trails across the lawn show where the sidewalks *should* have been built.

These are desire paths: physical traces of actual human behavior, revealing gaps between designed infrastructure and real needs.

**dp** brings this concept to AI coding assistants. When Claude Code, Cursor, or any AI tool hallucinates a function that doesn't exist, when it invokes a tool that isn't available, when it fails trying to use capabilities it wishes it had—those failures are signals. They're desire paths in your workflow, pointing to features that should exist.

dp captures these failed tool calls, aggregates them into patterns, and surfaces the most common ones. Instead of watching the same errors scroll past day after day, you can see what your AI really needs, find the closest matches in its actual toolset, and wire up aliases to fix the gap.

## Quick Demo

Here's the workflow:

```bash
# Install
go install github.com/scbrown/desire-path/cmd/dp@latest

# Connect to Claude Code
dp init --source claude-code

# Work normally in Claude Code; desires accumulate automatically
# ...some time passes...

# See the failures
dp list

# View aggregated patterns ranked by frequency
dp paths

# Inspect a specific pattern
dp inspect read_file

# Find close matches among known tools
dp suggest read_file

# Wire up the fix
dp alias read_file Read
```

Done. Now when your AI tries to call `read_file`, it gets routed to `Read`. The desire path becomes a real sidewalk.

## What It Does

- **Captures failures**: Hook into AI tool output streams to record every failed tool invocation
- **Finds patterns**: Aggregate similar failures into paths ranked by frequency
- **Suggests fixes**: Use Levenshtein-based similarity to match hallucinated tools to real ones
- **Creates aliases**: Map the hallucinated names to actual tools, fixing the gap
- **Tracks everything**: Optional full invocation logging for deeper analysis (success + failure)

## What It Doesn't Do

dp is not a proxy, not a wrapper, not a runtime interceptor. It doesn't sit between your AI and its tools. It's a passive observer and a pattern analyzer. You run it once to set up hooks, then it watches quietly and builds a database of desire paths. When you're ready, you query that database and act on the insights.

## Why This Matters

AI coding assistants evolve fast. Their tool sets change, their output formats shift, and they constantly hallucinate new capabilities before those capabilities actually exist. Instead of treating these failures as noise, dp treats them as signal. Every failed tool call is a vote for a feature request. dp counts the votes.

## Get Started

Ready to map your desire paths? Head to [Getting Started](./getting-started.md) to install dp and hook it into your AI tool.
