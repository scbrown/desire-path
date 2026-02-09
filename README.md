
<p align="center">
<pre align="center">
     ╺━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸

        ┏━━━┓
        ┃ ╱ ┃   ┏┳┓┏━╸┏━┓╻┏━┓┏━╸   ┏━┓┏━┓╺┳╸╻ ╻
        ┃╱  ┃   ┃┃┃┣╸ ┗━┓┃┣┳┛┣╸    ┣━┛┣━┫ ┃ ┣━┫
        ┗━━━┛   ╹╺┛┗━╸┗━┛╹╹┗╸┗━╸   ╹  ╹ ╹ ╹ ╹ ╹

      <i>Where the AI walks, the sidewalk should follow.</i>

     ╺━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╸
</pre>
</p>

<p align="center">
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
  <a href="https://goreportcard.com/report/github.com/scbrown/desire-path"><img src="https://goreportcard.com/badge/github.com/scbrown/desire-path" alt="Go Report Card"></a>
  <img src="https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white" alt="Go 1.24+">
  <img src="https://img.shields.io/badge/SQLite-embedded-003B57?logo=sqlite&logoColor=white" alt="SQLite">
  <img src="https://img.shields.io/badge/CGo-none-success" alt="No CGo">
  <img src="https://img.shields.io/badge/deps-4-brightgreen" alt="4 Dependencies">
</p>

---

AI coding assistants hallucinate tool names. They call `read_file` when they mean `Read`. They try `run_tests` when `Bash` is right there. Every failed tool call is a **signal** — it reveals what the AI *expected* to work.

**`dp` captures those signals, finds the patterns, and helps you fix them.**

Think of it like [desire paths](https://en.wikipedia.org/wiki/Desire_path) on a university campus — those worn trails through the grass where people actually walk. You don't fight the path. You pave it.

---

## 🎬 See It In Action

```bash
# 1. Hook into Claude Code (one-time setup)
$ dp init --source claude-code
✓ Configured Claude Code integration
  Hook: PostToolUseFailure → dp record --source claude-code

# 2. Use Claude Code normally... failures get recorded silently in the background

# 3. See what's been happening
$ dp list --since 7d
TIMESTAMP              SOURCE        TOOL              ERROR
2026-02-08 14:23:01    claude-code   read_file         unknown tool "read_file"
2026-02-08 14:25:33    claude-code   read_file         unknown tool "read_file"
2026-02-08 15:01:12    claude-code   execute_command   unknown tool "execute_command"
2026-02-07 09:44:55    claude-code   search_files      unknown tool "search_files"
2026-02-07 09:45:02    claude-code   search_files      unknown tool "search_files"
2026-02-06 11:30:18    claude-code   write_file        unknown tool "write_file"
...

# 4. Find the patterns — what's failing most?
$ dp paths --top 5
RANK  PATTERN            COUNT  FIRST_SEEN  LAST_SEEN   ALIAS
1     read_file          47     2026-01-15  2026-02-08
2     search_files       23     2026-01-18  2026-02-07
3     execute_command    18     2026-01-20  2026-02-08
4     write_file         12     2026-01-22  2026-02-06
5     list_directory      8     2026-02-01  2026-02-05

# 5. What real tool does "read_file" look like?
$ dp suggest read_file
RANK  TOOL    SCORE
1     Read    0.72
2     Grep    0.41

# 6. Lock it in — map the hallucination to reality
$ dp alias read_file Read
✓ Alias set: read_file → Read
```

> 💡 **47 failures from one hallucinated name.** Now you know exactly what to build — or alias — next.

---

## ✨ Features

🔍 **Pattern Detection** — Aggregate failures into ranked paths. See what's breaking most, not just what broke last.

🧠 **Smart Suggestions** — Levenshtein-powered similarity engine finds the real tool name behind every hallucination. CamelCase and underscore-aware.

🔗 **Alias System** — Map `read_file` → `Read` once, see it everywhere. Upsert behavior, instant updates.

🔌 **Plugin Architecture** — Extensible source plugins. Claude Code ships built-in. Write your own in ~50 lines of Go.

📊 **Full Telemetry** — Track *all* tool calls (not just failures) with `--track-all`. Success rates, usage patterns, session timelines.

💾 **Zero-Config Storage** — Embedded SQLite, pure Go, no CGo. Just works. Single file at `~/.dp/desires.db`.

📤 **Export Anything** — JSON, CSV, pipe to `jq`. Your data, your way.

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
| `dp suggest` | Find similar known tools via string similarity |
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

> 🔌 **Want to add your tool?** The plugin interface is ~50 lines of Go. See [Writing a Source Plugin](docs/book/src/integrations/writing-plugins.md).

---

## 🏗️ How It Works

```mermaid
graph LR
    A[AI Tool] -->|hook payload| B[Source Plugin]
    B -->|Extract| C[Universal Fields]
    C -->|Ingest| D[SQLite]
    D -->|Query| E[dp list / paths / stats]
    E -->|Analyze| F[dp suggest / inspect]
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

Config lives at `~/.dp/config.json`. See the [Configuration Reference](docs/book/src/configuration.md) for all options.

---

## 📖 Documentation

Full documentation is available in the [docs](docs/book/src/SUMMARY.md):

- **[Introduction](docs/book/src/introduction.md)** — The what and why
- **[Getting Started](docs/book/src/getting-started.md)** — Zero to insights in 5 minutes
- **[Concepts](docs/book/src/concepts/README.md)** — Desires, paths, aliases, invocations
- **[Command Reference](docs/book/src/commands/README.md)** — Every command, every flag
- **[Integrations](docs/book/src/integrations/README.md)** — Claude Code setup, plugin authoring
- **[Architecture](docs/book/src/architecture.md)** — Data model, storage, plugin system

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
