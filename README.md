<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/assets/banner-dark.svg">
    <source media="(prefers-color-scheme: light)" srcset="docs/assets/banner-light.svg">
    <img alt="Desire Path" src="docs/assets/banner-dark.svg" width="660">
  </picture>
</p>
<h1 align="center">Desire Path</h1>
<p align="center"><em>👣 Find the tool calls your agents get wrong, and turn repeats into fixes.</em></p>
<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="License: MIT"></a>
  <a href="https://github.com/scbrown/desire-path/actions/workflows/ci.yml"><img src="https://github.com/scbrown/desire-path/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/scbrown/caboodle"><img src="https://img.shields.io/badge/stack-quipu-8B5E3C.svg" alt="Quipu stack"></a>
</p>

**Desire Path (`dp`) is a CLI and a set of agent hooks for developers who want
fewer repeated tool-call failures. It stores calls locally, ranks recurring
failures, and records mappings from mistaken tool names to real ones.**
The name comes from the trails people wear through grass: evidence of where a path is needed.

## Why you would want it

- Find repeated failures across sessions instead of reading each transcript.
- Compare mistaken tool names with known tools and record the intended mapping.
- Use the failure history to improve instructions, tool names, or integrations.

Read [how the workflow fits together](docs/book/src/introduction.md).

## Install

Linux x86-64, pinned release with checksum verification:

```bash
mkdir -p /tmp/dp-install && cd /tmp/dp-install
curl -fLO https://github.com/scbrown/desire-path/releases/download/v0.2.1/desire-path_0.2.1_linux_amd64.tar.gz
curl -fLO https://github.com/scbrown/desire-path/releases/download/v0.2.1/checksums.txt
sha256sum --check --ignore-missing checksums.txt
tar -xzf desire-path_0.2.1_linux_amd64.tar.gz dp
mkdir -p "$HOME/.local/bin" && install -m 755 dp "$HOME/.local/bin/dp"
export PATH="$HOME/.local/bin:$PATH"
dp version
```

Expected: `dp 0.2.1 (6c5840f)`. If you see something else, check `command -v dp`.

From source (Go 1.24+): `go install github.com/scbrown/desire-path/cmd/dp@v0.2.1`,
then add `$(go env GOPATH)/bin` to PATH and run `dp version`.
See [installation](docs/book/src/getting-started.md) for macOS, Windows, and source-checkout builds.

## First success in three commands

Use a temporary database and one synthetic failed call; Python 3 formats the result:

```bash
DP_DEMO=$(mktemp -d)
printf '%s\n' '{"tool_name":"read_file","error":"unknown tool","session_id":"demo"}' | dp --db "$DP_DEMO/desires.db" ingest --source claude-code --json > "$DP_DEMO/record.json"
dp --db "$DP_DEMO/desires.db" paths --json | python3 -c 'import json,sys; p=json.load(sys.stdin)[0]; print(p["pattern"], p["count"])'
```

Expected stdout:

```text
read_file 1
```

The failure became a queryable pattern without configuring an agent or touching your usual database.
An optional-metrics diagnostic may also appear on stderr; see [troubleshooting](docs/book/src/getting-started.md#troubleshooting).

## On your own data

After your agent has recorded calls:

| question | command |
|---|---|
| Which failures repeat? | `dp paths --top 5` |
| What happened for this tool? | `dp inspect read_file` |
| What tool might have been intended? | `dp similar read_file` |
| How do I record the intended mapping? | `dp alias read_file Read` |
| How do I export the failures? | `dp export --format json` |

An alias records a mapping. Active interception is a separate [pave setup](docs/book/src/commands/pave.md).
See the [CLI reference](docs/book/src/commands/README.md) for flags and all commands.

## Wire it into your agent

For Claude Code, with `dp` on the agent's PATH:

```bash
dp init --source claude-code
dp sources
```

The installer merges hooks into `~/.claude/settings.json`. It installs ingestion
for successes and failures, plus signposting and correction hooks. Review that
file after installation. [Using agents](docs/book/src/integrations/README.md)
explains the installed hooks and the Codex, Cursor, and Kiro source plugins.

## Before you start

| requirement | support |
|---|---|
| Release archives | Linux, macOS, Windows; amd64 and arm64 |
| Source build | Go 1.24+; no CGo required |
| README demo | Bash-compatible shell and Python 3 |
| Local storage | SQLite at `~/.dp/desires.db`; override with `--db` |
| Hook capture | A supported agent and `dp` on its PATH |

Keep tool inputs and error messages in mind when sharing [exports](docs/book/src/commands/export.md).

## What's next

- [Read the book](docs/book/src/introduction.md)
- [Find every design and reference document](docs/book/src/docs-map.md)
- [Configure capture for your agent](docs/book/src/integrations/README.md)

## 🧺 The stack

Caboodle installs these together and proves each one works; every tool also stands alone.

| tool | what it gives your agents |
|---|---|
| [caboodle](https://github.com/scbrown/caboodle) | one wizard that installs the stack and proves it works |
| [quipu](https://github.com/scbrown/quipu) | a knowledge graph that refuses facts that break its rules |
| [camayoc](https://github.com/scbrown/camayoc) | the starter vocabulary, and how new knowledge earns its way in |
| [bobbin](https://github.com/scbrown/bobbin) | search and context over your repositories, served over MCP |
| [yupana](https://github.com/scbrown/yupana) | which code calls which: the blast radius before an edit |
| [desire-path](https://github.com/scbrown/desire-path) **(you are here)** | the tool calls your agents get wrong, so you can fix them |

## Contributing

From a checkout, with [just](https://github.com/casey/just), Go, Python 3,
pre-commit, and mdBook installed:

```bash
just build
just test
just check
```

See the [development guide](book/src/development/contributing.md).

## 📜 License

[MIT](LICENSE).
