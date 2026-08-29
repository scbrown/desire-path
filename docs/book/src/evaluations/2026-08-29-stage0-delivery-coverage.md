# Stage 0: delivery coverage, certified with no model in the loop

**Date:** 2026-08-29
**Provider quota spent:** zero — no model runs in this measurement
**Harness:** `eval/replay-delivery.sh` over `eval/replay-workload.jsonl` (185 rows)
**Index:** `eval/build-local-index.sh` over the five pinned corpora in `eval/corpora.json`

## Result

Against a resident local index holding all five pinned corpora, signpost
delivery is **not the bottleneck**. Four arms, every eligible event delivered:

| arm | eligible | shown | coverage | timed out | zero candidate | warm hits |
|---|--:|--:|--:|--:|--:|--:|
| `always-signpost` direct | 185 | 185 | **100%** | 0 | 0 | 0 |
| `gated-signpost` direct | 35 | 35 | **100%** | 0 | 0 | 0 |
| `always-signpost` prefetch 500 ms | 185 | 185 | **100%** | 0 | 0 | 185 |
| `gated-signpost` prefetch 500 ms | 35 | 35 | **100%** | 0 | 0 | 35 |

The Stage 1 gate was coverage >= 25%. It is met with room, and the campaign's
published 6.5% is explained without appealing to retrieval quality.

Latency on the two direct arms, from the committed events: p50 **92 ms**
(always) and **97 ms** (gated), max **134 ms**, against the 150 ms PostToolUse
contract. The prefetch arms record no lookup latency at all — every event was a
warm cache hit, so the PostToolUse hook made no request.

A deliberately cold arm — server restarted, replay begun as soon as it accepted
a connection — also delivered 185/185, with the first 20 events at p50 126 ms
against 93 ms for the rest and a maximum of 136 ms. **Delivery survives a cold
start, but the headroom at that maximum is ~14 ms, so this is a result about a
settled server on this hardware and not a portable guarantee.**

An earlier direct arm delivered 34 of 35 with one event landing exactly on the
150 ms boundary. Coverage on the direct path is therefore ~97-100%, not a flat
100%; the prefetch arms had no such event.

## What the earlier campaign was actually measuring

Two independent causes, neither of them retrieval quality:

1. **The prefetch head start, not the commit.** `9c23bd4` only adds the
   `DP_SIGNPOST_PREFETCH=0` opt-out; the published run disabled prefetch by
   environment. Disabling it collapses the effective semantic budget from the
   5 s concurrent prefetch to the 150 ms direct contract while leaving that
   contract's number unchanged, which is why the drop looked like a code
   regression.

2. **Backend corpus coverage, which fixes the "zero-candidate floor" in place.**
   The shared fleet search backend does not index every pinned corpus. Measured
   with controls in both directions, one query, `limit=3`:

   | repo filter | count |
   |---|--:|
   | `desire-path` | 3 |
   | `gastown` | 3 |
   | `beads` | 3 |
   | `cobra` | **0** |
   | `ripgrep` | **0** |
   | `desire_path` (deliberate misspelling, negative control) | **0** |

   Two of the five corpora are absent from that index, and 33 of the 185
   workload rows (17.8%) are in them. An event in an unindexed repo returns no
   candidate no matter how much latency budget it is given, and it is
   **indistinguishable in the logs from a semantic miss**. The negative control
   shows the same query returns 0 for a repo name that does not exist, which is
   what a name mismatch would also produce.

   Against the local index built from the corpora themselves, zero-candidate
   events are 0 of 185.

## The honest limit of a coverage number

Coverage says a signpost reached the model. It says nothing about whether the
signpost was worth reading, and this harness cannot tell you: the backend
returns nearest neighbours, so a deliberately nonsensical query
(`zzqq flurbleplex nonexistent tokenoid`) also returns 3 results. With no
relevance floor, a "candidate exists" predicate is satisfied almost everywhere,
and 100% coverage is the expected reading rather than a strong one.

So Stage 0 certifies exactly one thing: **delivery is no longer the constraint,
and a campaign that spends model quota will now actually exercise the
intervention.** Adoption and correctness remain unmeasured, and the two pilots
that did measure them found zero adoptions.

## Reproducing

```sh
eval/build-local-index.sh --corpus-root <dir of the five pinned checkouts> --index <dir>
ORT_DYLIB_PATH=$HOME/.local/lib/onnxruntime/libonnxruntime.so bobbin serve --http --port 3131 <dir>
eval/replay-delivery.sh --dp ./dp --bobbin-server http://127.0.0.1:3131/search \
    --out eval/results --condition gated-signpost
```

`build-local-index.sh` carries the ONNX loader recipe that the local-warm pilot
worked around without writing down: `ort` dlopens `libonnxruntime.so` by bare
name and panics if the loader cannot find it, and `ORT_DYLIB_PATH` must name a
**real file**, not a directory and not a bare soname. The script refuses to run
if it does not, because a missing dylib otherwise indexes nothing and the empty
corpus reads as a genuine coverage measurement of zero.

## Artifacts

Event-level JSONL, one row per replayed hook invocation, is committed beside the
summaries — the pilots wrote theirs to a temp directory, which is why analysing
them needed archaeology:

- `eval/results/2026-08-29-stage0-events-<arm>.jsonl`
- `eval/results/2026-08-29-stage0-coverage-<arm>.json`
- `eval/results/2026-08-29-stage0-summary.json`

### A defect in the first cut of this harness, worth knowing when reading older arms

`eligible` originally counted every replayed row. Under `gated-signpost` most
rows never reach the searcher, so they were scored as eligible-but-undelivered:
the first gated arm read **33/185 = 18%** for a run that had delivered 33 of 35,
**94%**. Eligibility is now derived from the marks a lookup actually leaves — a
shown signpost, a warm cache hit, or a nonzero latency — rather than from the
`predicate` field, which records the literal-search outcome and is `none` for
rows that `always-signpost` searches anyway.

Arm names now carry the condition too. They did not, so a gated run silently
overwrote an always-signpost run's events in the same output directory.
