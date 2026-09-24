# Getting started

Install a release, ingest one synthetic failure, and query it before connecting an agent.

## Install a release

For Linux x86-64:

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

[Release v0.2.1](https://github.com/scbrown/desire-path/releases/tag/v0.2.1)
also contains Linux arm64, macOS amd64/arm64, and Windows amd64/arm64 archives.
Download your matching archive and `checksums.txt`, verify its SHA-256 before
extracting, then place `dp` (or `dp.exe`) on PATH. On macOS use `shasum -a 256`
to compare the archive digest with its entry; on Windows use `Get-FileHash -Algorithm SHA256`.
The commands above specifically exercise the Linux amd64 archive.

## Build from source

With Go 1.24 or later:

```bash
go install github.com/scbrown/desire-path/cmd/dp@v0.2.1
export PATH="$(go env GOPATH)/bin:$PATH"
dp version
```

A Go module install does not receive the release build's version linker flags;
its output can say `dev`. To build the current checkout with Git metadata:

```bash
git clone https://github.com/scbrown/desire-path.git
cd desire-path
make install
dp version
```

## First success in three commands

This uses a separate temporary database and requires Python 3:

```bash
DP_DEMO=$(mktemp -d)
printf '%s\n' '{"tool_name":"read_file","error":"unknown tool","session_id":"demo"}' | dp --db "$DP_DEMO/desires.db" ingest --source claude-code --json > "$DP_DEMO/record.json"
dp --db "$DP_DEMO/desires.db" paths --json | python3 -c 'import json,sys; p=json.load(sys.stdin)[0]; print(p["pattern"], p["count"])'
```

Expected stdout:

```text
read_file 1
```

`ingest` records the invocation and, because the payload has an error, its failure
pattern. The saved `record.json` carries the generated ID and timestamp. Run the
three commands again to get another isolated database and the same output.

## Connect your agent

Follow [Using agents](integrations/README.md). Once calls accumulate, try
`dp paths --top 5`, `dp inspect read_file`, and `dp similar read_file`.
`dp alias read_file Read` records an intended mapping; it does not by itself
install interception. See [pave](commands/pave.md) for that separate setup.

## Troubleshooting

- **Wrong version:** check `command -v dp`. A different binary may appear earlier on PATH.
- **Metrics diagnostic:** without `DESIRE_PATH_METRICS_PUSHGATEWAY`, v0.2.1 reports
  that no metrics were pushed on stderr. Local ingestion still works; check the query result.
- **No failures yet:** `dp paths` needs failed invocations. Use the synthetic fixture above
  to distinguish an empty history from a broken setup.
- **Hook not found:** the agent process needs the same PATH that contains `dp`.

See [configuration](configuration.md) for persistent settings and database paths.
