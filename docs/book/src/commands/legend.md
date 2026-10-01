# Read-time entity legend (dp legend-refresh)

The entity legend tells an agent which Quipu entities the text it just read
refers to. After a `Read`, or a Bash `cat`/`head`/`tail`/`sed -n`/`br show`,
the PostToolUse hook can add one line:

```text
Quipu entities here: build-01.example (Host), search-webhook (SystemdService), store-recycle (ambiguous 2)
```

The agent then knows what the graph covers without having to search for it.

It is **off by default**. Set `DP_LEGEND=1` to enable it.

## How it fits the hook

The legend is a second stage of `dp signpost`, not a separate hook. The two
stages share one per-call context budget (`DP_LEGEND_MAX_BYTES`, default
600 B):

- The signpost stage runs first. If it emits, it keeps its bytes, because it
  answers the search the agent actually ran.
- The legend gets whatever remains, which can be nothing.
- If signpost used the whole budget, its output is passed through
  byte-identical.

The hook path makes **no network call**. It reads a local gazetteer file that
`dp legend-refresh` writes on a timer. The stage stays silent and leaves the
hook output exactly as it was in any of these cases:

- the gazetteer is missing, unreadable, or older than `DP_LEGEND_MAX_AGE_MIN`
  (default 1440 minutes)
- there is no budget left
- there is no match
- anything else goes wrong

A stale legend could name entities that have since been renamed or retired,
and a confident wrong annotation is worse than none.

## What gets matched

`dp legend-refresh` exports `rdfs:label` values for a fixed list of governed
classes, such as Host, Service, GitRepo, Directive, FailureMode and AlertRule.
Generated and bulk kinds (Observation, Chunk, Commit, CodeSymbol, Section) are
excluded, as are Credential and Formula. Labels are admitted only if they pass
these precision guards:

- at least 4 characters
- a single token must be identifier-shaped: it contains `-`, `_`, `.`, `/`
  or a digit, or is CamelCase
- a single token must not be a dictionary word
- a multi-word label must not consist only of dictionary words and stopwords
- a short stoplist is dropped: ubiquitous URL hosts (`github.com`,
  `crates.io`, ...) and names that are also everyday phrases or generic filenames (`code-review`, `deploy.yml`).
  A phrase is added only when a hand-checked sample shows it misleading.

At read time, one Aho-Corasick pass matches every label case-insensitively
with these rules:

- matches respect word boundaries
- the longest label wins at any position
- a label followed by `.<ext>` is rejected, because it is a filename stem
  rather than a mention
- each label is reported once per read
- a label that names several IRIs is shown as `ambiguous (n)`
- a bead or file never annotates itself

Each read shows at most 5 entities, and an entity already shown earlier in the
session is skipped.

## Environment

| Variable | Default | Meaning |
|---|---|---|
| `DP_LEGEND` | off | `1` enables the stage |
| `DP_LEGEND_GAZETTEER` | `<user cache>/desire-path/legend-gazetteer.json` | gazetteer path |
| `DP_LEGEND_QUIPU_URL` | required | Quipu `/query` endpoint used by refresh |
| `DP_LEGEND_NAMESPACE` | required | ontology namespace the class names live in |
| `DP_LEGEND_MAX_BYTES` | 600 | budget combined with the signpost stage |
| `DP_LEGEND_MAX_AGE_MIN` | 1440 | refuse an older gazetteer |
| `DP_LEGEND_LOG` | `~/.local/log/dp-legend-events.jsonl` | decision log (`-` disables) |
| `DP_LEGEND_DICT` | `/usr/share/dict/words` | word list for the guard |

## Decision log

Every read the stage sees appends one JSON line with the following fields:

- `tool` and `ref`
- `text_bytes` and `raw_hits`
- the `shown` labels
- `bytes` and `budget`
- `latency_us`, which includes `load_us`, the time to read and compile the
  gazetteer
- `skipped`, the reason for silence

The log is separate from the signpost event log, so Read events do not dilute
the signpost evaluation rows. It is also the data source for the value metric:
the share of shown entities the agent later queries or cites.

## Refreshing

```bash
dp legend-refresh            # writes the gazetteer atomically, reports each class on stderr
```

Run it from a timer, for example hourly. If every class query fails, refresh
exits non-zero and leaves the existing file in place, so a Quipu outage never
replaces a good gazetteer with an empty one.
