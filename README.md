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

Read [how the workflow fits together](https://scbrown.github.io/desire-path/introduction.html).

## Install

**Debian and Ubuntu (amd64 and arm64):** install from the signed apt repository, which
republishes each checksum-verified release. The package puts `dp` in
`/usr/bin`. Check the signing key's fingerprint before trusting it; the
commands stop if it does not match:

```bash
curl -fsSL https://scbrown.github.io/apt/key.gpg -o scbrown-stack.gpg
gpg --show-keys --with-colons scbrown-stack.gpg \
  | grep -qx 'fpr:::::::::AB5625C5612F643BF76464DF0DC99924994EC959:' \
  && sudo install -D -m 0644 scbrown-stack.gpg /etc/apt/keyrings/scbrown-stack.gpg \
  && echo "deb [signed-by=/etc/apt/keyrings/scbrown-stack.gpg] https://scbrown.github.io/apt stable main" \
     | sudo tee /etc/apt/sources.list.d/scbrown-stack.list \
  && sudo apt update && sudo apt install desire-path
```

`apt upgrade` then picks up new releases. unattended-upgrades only installs
from origins it allows, so to have it upgrade desire-path too, add
`"origin=scbrown,codename=stable";` to `Unattended-Upgrade::Origins-Pattern`.
Channels, pinning and rollback:
[scbrown/apt](https://github.com/scbrown/apt#channels-and-pinning).

Linux x86-64, pinned release with checksum verification:

```bash
mkdir -p /tmp/dp-install && cd /tmp/dp-install
curl -fLO https://github.com/scbrown/desire-path/releases/download/v0.7.0/desire-path_0.7.0_linux_amd64.tar.gz
curl -fLO https://github.com/scbrown/desire-path/releases/download/v0.7.0/checksums.txt
sha256sum --check --ignore-missing checksums.txt
tar -xzf desire-path_0.7.0_linux_amd64.tar.gz dp
mkdir -p "$HOME/.local/bin" && install -m 755 dp "$HOME/.local/bin/dp"
export PATH="$HOME/.local/bin:$PATH"
dp version
```

Expected: `dp 0.7.0 (5ca33ba)`. If you see something else, check `command -v dp`.

From source (Go 1.24+): `go install github.com/scbrown/desire-path/cmd/dp@v0.7.0`,
then add `$(go env GOPATH)/bin` to PATH and run `dp version`.
See [installation](https://scbrown.github.io/desire-path/getting-started.html) for macOS, Windows, and source-checkout builds.

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
An optional-metrics diagnostic may also appear on stderr; see [troubleshooting](https://scbrown.github.io/desire-path/getting-started.html#troubleshooting).

## On your own data

After your agent has recorded calls:

| question | command |
|---|---|
| Which failures repeat? | `dp paths --top 5` |
| What happened for this tool? | `dp inspect read_file` |
| What tool might have been intended? | `dp similar read_file` |
| How do I record the intended mapping? | `dp alias read_file Read` |
| How do I export the failures? | `dp export --format json` |

An alias records a mapping. Active interception is a separate [pave setup](https://scbrown.github.io/desire-path/commands/pave.html).
See the [CLI reference](https://scbrown.github.io/desire-path/commands/) for flags and all commands.

## Wire it into your agent

For Claude Code, with `dp` on the agent's PATH:

```bash
dp init --source claude-code
dp sources
```

The installer merges hooks into `~/.claude/settings.json`. It installs ingestion
for successes and failures, plus signposting and correction hooks. Review that
file after installation. [Using agents](https://scbrown.github.io/desire-path/integrations/)
explains the installed hooks and the Codex, Cursor, and Kiro source plugins.

## Before you start

| requirement | support |
|---|---|
| Release archives | Linux, macOS, Windows; amd64 and arm64 |
| Source build | Go 1.24+; no CGo required |
| README demo | Bash-compatible shell and Python 3 |
| Local storage | SQLite at `~/.dp/desires.db`; override with `--db` |
| Hook capture | A supported agent and `dp` on its PATH |

Keep tool inputs and error messages in mind when sharing [exports](https://scbrown.github.io/desire-path/commands/export.html).

## What's next

- [Read the book](https://scbrown.github.io/desire-path/introduction.html)
- [Find every design and reference document](https://scbrown.github.io/desire-path/docs-map.html)
- [Configure capture for your agent](https://scbrown.github.io/desire-path/integrations/)

## 🧺 The stack

Caboodle installs these together and proves each one works; every tool also stands alone.

<!-- stack-table:start -->

The Quipu stack is built around a governed, bitemporal knowledge store: shared
facts, decisions and work history your agents can query. Quipu can check writes
against your rules, records where each fact came from, and answers what was true
when.

| tool | what it gives your agents |
|---|---|
| [caboodle](https://github.com/scbrown/caboodle) | 🧰 Agent Platform Toolkit - installs and configures the quipu stack |
| [quipu](https://github.com/scbrown/quipu) | stores shared facts with provenance and history; can check writes against your rules |
| [camayoc](https://github.com/scbrown/camayoc) | prepares knowledge and vocabulary for Quipu, with checks before it is admitted |
| [bobbin](https://github.com/scbrown/bobbin) | searches your code and can include related knowledge from Quipu |
| [yupana](https://github.com/scbrown/yupana) | shows code impact and can read policies from Quipu to check changes |
| [desire-path](https://github.com/scbrown/desire-path) | turns agent mistakes into feature requests; works on its own without Quipu |
| [seeds](https://github.com/scbrown/seeds) | a work item tracker backed by the Quipu knowledge base |
| [shantytown](https://github.com/scbrown/shantytown) | runs crews of AI agents on work items, triggered by hand, on a schedule or by events; can share work and memory through Quipu |
| [shuttle](https://github.com/scbrown/shuttle) | runs signed agent workflows and keeps their records in Quipu |

<!-- stack-table:end -->

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

Repeated failures can be measured with `dp failure-families --since <RFC3339> --until <RFC3339> --window 1h --json`. It groups by tool, parsed shell command-program shape and a bounded error class; compound commands are grouped as programs, without claiming which subcommand failed. Argument values, paths and error messages are absent from output. Unsupported wrappers and dynamic command identities are counted separately as UNKNOWN. Synthetic turn-pattern observations are excluded. `peak_window` measures the historical rolling maximum; `current_window` measures the window ending at `--until`. These are observations, not an alerting policy or a claim of complete capture.

Local `failure-families` reads require an existing database at the current schema
version. The command opens it read-only: missing databases and schema mismatches
return an error without creating a store or migrating it. Configured remote
stores continue to use their existing read API.
