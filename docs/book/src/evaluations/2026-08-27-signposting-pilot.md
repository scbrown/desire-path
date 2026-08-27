# Signposting pilot: production latency budget

**Date:** 2026-08-27  
**Models:** Codex CLI 0.147.0 and Claude Code 2.1.247  
**Design:** 7 pinned tasks × 2 model families × 5 conditions  
**Runs:** 70/70 completed, one replicate, no failed artifacts

## Result

At the production 150 ms semantic timeout, signposting delivered no
intervention. The hooks observed 78 Bash literal searches. Forty-seven were
eligible for a semantic request (all searches in always-signpost plus null or
high-cardinality searches in gated mode), but all 47 timed out and zero
signposts were shown.

This pilot therefore supports no adoption, signpost-precision, correctness, or
resource-efficiency claim for signposting. The gated and always-on cells were
operationally equivalent to their literal-search baseline because the semantic
side never reached the model.

A separately labeled 10-second sensitivity probe established that the hook
path itself works: a repo-scoped Bobbin request returned three candidates after
2.275 seconds and the Codex model received the signpost. The model did not adopt
it on that single probe. This sensitivity result is not included in the 70-run
summary.

## Reproducibility checks

- All five corpus directories were checked out at the SHA declared in
  `eval/corpora.json` before execution.
- The assignment file contained 70 unique IDs and exactly seven rows for every
  model-family/condition cell.
- Both agents ran read-only. The runner retained one raw JSONL transcript and
  one hook-event stream per assignment outside the repository.
- The committed result file has 70 rows; the scorer produced ten summary cells.
- Four replacement-arm rows from an early shim that intercepted shell-startup
  probes were invalidated and rerun after the shim was narrowed to the explicit
  evaluation command. Invalidated artifacts were not mixed into these results.

## Secondary observations

Prompt instruction produced eight direct Bobbin invocations (six Claude, two
Codex). They are not signpost adoption because no signpost was shown. The raw
exact-location scorer marked 36 of 70 final answers correct, but this figure is
not suitable for a paper claim: post-run audit found acceptable nearby or
caller locations that were absent from several single-location fixtures. The
fixtures and adjudicator need a blinded acceptance-set pass before correctness
comparisons are reported.

Runtime cost also varied substantially by family. Claude averaged roughly
205k–226k recorded tokens per cell-run depending on condition and 4.9–7.1 Bash
turns; Codex averaged roughly 73k–111k tokens and 2.3–3.6 Bash turns. With one
replicate and seven tasks, these are pilot diagnostics, not general estimates.

## Decision

Do not expand to several hundred tasks or draft the planned efficacy preprint
from this dataset. First choose and measure a latency remedy that preserves the
timeout-and-drop contract—such as a local/warm index or speculative prefetch—then
rerun the same pilot. Independently, replace exact-string final-answer scoring
with blinded acceptance sets before consuming another full campaign budget.

Artifacts:

- `eval/results/2026-08-27-pilot-results.jsonl`
- `eval/results/2026-08-27-pilot-summary.json`
- `eval/run-campaign.sh`
