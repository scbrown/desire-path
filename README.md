<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/banner-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="docs/assets/banner-light.svg">
    <img alt="desire path" src="docs/assets/banner-dark.svg" width="660">
  </picture>
</p>

<p align="center">
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white" alt="Go 1.24+">
  <img src="https://img.shields.io/badge/SQLite-embedded-003B57?logo=sqlite&logoColor=white" alt="SQLite">
  <img src="https://img.shields.io/badge/CGo-none-success" alt="No CGo">
  <img src="https://img.shields.io/badge/deps-4-brightgreen" alt="4 Dependencies">
</p>

---

Claude calls `Edit` with `file` instead of `file_path`. The tool rejects it. Claude retries with the right parameter, burning tokens and time. Tomorrow it makes the same mistake. Next week, 23 more times. Across your team, hundreds of wasted retries — all from the same mismatch.

**`dp` captures every failed tool call, surfaces the patterns, and shows you exactly what to fix — or what to build next.**

Think of it like [desire paths](https://en.wikipedia.org/wiki/Desire_path) on a campus — worn trails through the grass where people actually walk. You don't fight the path. You pave it.

---

## Signposting (private sibling mode)

**Signposting** executes a deterministic literal search unmodified and then,
only when its result is empty or too large, offers a **signpost**: a pointer and
runnable Bobbin command on the agent hook channel. The agent's subsequent use
of that command is **adoption**. The null/high-cardinality condition is the
**gating predicate**. **Contract preservation** means the original command's
stdout and exit status remain byte-identical to the un-intercepted baseline.

The mode is a sibling to hallucination capture: it observes completed
Bash-mediated `grep` and `rg` calls through PostToolUse and never shadows either
binary. Native agent Grep/Glob tools are outside v1. Semantic timeouts and
errors are dropped, leaving baseline behavior unchanged. See
[`docs/plans/009-signposting-eval.md`](docs/plans/009-signposting-eval.md) for
the eval-first event contract.

---

## 🎬 See It In Action

```bash
# 1. Hook into Claude Code (one-time setup)
$ dp init --source claude-code
✓ Configured Claude Code hooks
  PostToolUseFailure → dp record --source claude-code

# 2. Use Claude Code normally — failures get recorded in the background

# 3. A week later: what's been happening?
$ dp paths --top 5
RANK  PATTERN            COUNT  FIRST_SEEN  LAST_SEEN
1     search_files       47     2026-01-15  2026-02-08
2     Edit:file          23     2026-01-18  2026-02-07
3     execute_command    18     2026-01-20  2026-02-08
4     write_file         12     2026-01-22  2026-02-06
5     list_directory      8     2026-02-01  2026-02-05

# 4. "search_files" keeps failing — what real tool is it closest to?
$ dp similar search_files
CANDIDATE  SCORE
Grep       0.82
Glob       0.64

# 5. Map the hallucination to the real tool
$ dp alias search_files Grep
✓ Alias: search_files → Grep

# 6. Or better — 47 sessions tried "search_files". That's a feature request.
#    Build an MCP tool called search_files that wraps Grep.
#    dp just told you where to pour the concrete.
```

> **47 failures, one pattern, one fix.** The AI is telling you what the tool *should* have been called. That's a desire path.

---

## ✨ Features

🔍 **Pattern Detection** — Aggregate failures into ranked paths. See what's breaking most, not just what broke last.

🧠 **Smart Suggestions** — Levenshtein-powered similarity engine finds the real tool name behind every hallucination. CamelCase and underscore-aware.

🔗 **[Alias System](https://scbrown.github.io/desire-path/concepts/aliases.html)** — Annotate patterns with what they *should* map to (`search_files` → `Grep`). Aliases appear in `dp paths` output and short-circuit `dp similar`. Use them to build your rosetta stone, then act on it — add instructions to CLAUDE.md, build MCP tool wrappers, or lobby for better tool names upstream.

🔌 **Plugin Architecture** — Extensible source plugins. Claude Code ships built-in. Write your own in ~50 lines of Go.

📊 **Full Telemetry** — Track *all* tool calls (not just failures) with `--track-all`. Success rates, usage patterns, session timelines.

💾 **Zero-Config Storage** — Embedded SQLite, pure Go, no CGo. Just works. Single file at `~/.dp/desires.db`.

📤 **[Export Anything](https://scbrown.github.io/desire-path/commands/export.html)** — Dump raw data as JSONL or CSV. Pipe through `jq` for ad-hoc analysis, feed into dashboards, or back up with `dp export > backup.jsonl`. Supports filtering by date and data type (failures vs all invocations).

🖥️ **Beautiful Output** — TTY-aware tables with bold headers. `--json` everywhere for scripting.

⚡ **Async & Silent** — Hook execution is async. dp never slows down your AI assistant.

🏗️ **Cross-Platform** — Linux, macOS, Windows. amd64 and arm64. Single binary, zero dependencies.

---

## 🚀 Quick Start

**60 seconds from install to insights:**

```bash
# Install
go install github.com/scbrown/desire-path/cmd/dp@latest

# Hook into your AI tool
dp init --source claude-code

# (Use Claude Code normally for a while...)

# What's failing?
dp paths

# Deep-dive a pattern
dp inspect read_file

# Fix it with an alias
dp alias read_file Read

# See your aliases
dp aliases
```

---

## 📦 Installation

### Go Install (recommended)

```bash
go install github.com/scbrown/desire-path/cmd/dp@latest
```

### From Source

```bash
git clone https://github.com/scbrown/desire-path.git
cd desire-path
make install
```

### Binary Releases

Pre-built binaries for Linux, macOS, and Windows available on the [Releases](https://github.com/scbrown/desire-path/releases) page.

---

## 🔧 Commands

### Record & Ingest

| Command | Description |
|---------|-------------|
| `dp record` | Record a failed tool call from stdin JSON |
| `dp ingest` | Ingest tool call data via a source plugin |
| `dp init` | Set up automatic recording from an AI tool |

### Query & Analyze

| Command | Description |
|---------|-------------|
| `dp list` | List recent desires with filtering |
| `dp paths` | Show aggregated patterns ranked by frequency |
| `dp inspect` | Deep-dive a specific pattern with histograms |
| `dp stats` | Summary statistics and activity overview |
| `dp export` | Export raw data as JSON or CSV |

### Map & Fix

| Command | Description |
|---------|-------------|
| `dp similar` | Find similar known tools via string similarity |
| `dp alias` | Create or update a tool name mapping |
| `dp aliases` | List all configured aliases |

### Configure

| Command | Description |
|---------|-------------|
| `dp config` | View or modify dp settings |

> 📖 Every command supports `--json` for machine-readable output and `--help` for details.

---

## 🔌 Integrations

### Claude Code ✅

```bash
# Failures only (default)
dp init --source claude-code

# Everything — failures AND successes
dp init --source claude-code --track-all
```

Hooks into Claude Code's `PostToolUseFailure` (and optionally `PostToolUse`) events. Async execution, zero impact on your workflow.

### Coming Soon 🚧

| Tool | Status |
|------|--------|
| Gemini CLI | Planned |
| Cursor | Planned |
| Kiro CLI | Planned |
| OpenCode | Planned |

> 🔌 **Want to add your tool?** The plugin interface is ~50 lines of Go. See [Writing a Source Plugin](https://scbrown.github.io/desire-path/integrations/writing-plugins.html).

---

## 🏗️ How It Works

```mermaid
graph LR
    A[AI Tool] -->|hook payload| B[Source Plugin]
    B -->|Extract| C[Universal Fields]
    C -->|Ingest| D[SQLite]
    D -->|Query| E[dp list / paths / stats]
    E -->|Analyze| F[dp similar / inspect]
    F -->|Fix| G[dp alias]

    style A fill:#e1bee7,stroke:#7b1fa2,color:#000
    style B fill:#bbdefb,stroke:#1565c0,color:#000
    style C fill:#c8e6c9,stroke:#2e7d32,color:#000
    style D fill:#fff9c4,stroke:#f9a825,color:#000
    style E fill:#ffccbc,stroke:#d84315,color:#000
    style F fill:#b3e5fc,stroke:#0277bd,color:#000
    style G fill:#dcedc8,stroke:#558b2f,color:#000
```

**Data flow**: Hook fires → source plugin parses the payload → universal fields extracted → stored in SQLite → query, analyze, and fix with the CLI.

---

## ⚙️ Configuration

```bash
# See all settings
dp config

# Change database location
dp config db_path /path/to/desires.db

# Default to JSON output
dp config default_format json

# Customize known tools for suggestions
dp config known_tools Read,Write,Edit,Bash,Glob,Grep,MyCustomTool
```

Config lives at `~/.dp/config.toml`. See the [Configuration Reference](https://scbrown.github.io/desire-path/configuration.html) for all options.

---

## 📖 Documentation

Full documentation available at **[scbrown.github.io/desire-path](https://scbrown.github.io/desire-path/)**:

- **[Introduction](https://scbrown.github.io/desire-path/introduction.html)** — The what and why
- **[Getting Started](https://scbrown.github.io/desire-path/getting-started.html)** — Zero to insights in 5 minutes
- **[Concepts](https://scbrown.github.io/desire-path/concepts/index.html)** — Desires, paths, aliases, invocations
- **[Command Reference](https://scbrown.github.io/desire-path/commands/index.html)** — Every command, every flag
- **[Integrations](https://scbrown.github.io/desire-path/integrations/index.html)** — Claude Code setup, plugin authoring
- **[Architecture](https://scbrown.github.io/desire-path/architecture.html)** — Data model, storage, plugin system

```bash
# Build the docs locally (requires mdbook)
make docs

# Serve with live reload
make docs-serve
```

---

## 🤝 Contributing

Contributions welcome! The plugin system is specifically designed for community extensions.

**Quick wins:**
- Add a source plugin for your favorite AI tool
- Report desire paths you've discovered (meta!)
- Improve documentation

---

## 📄 License

[MIT](LICENSE) — do what you want with it.

---

<p align="center">
  <i>Every failed tool call is a feature request from the future.</i>
</p>
