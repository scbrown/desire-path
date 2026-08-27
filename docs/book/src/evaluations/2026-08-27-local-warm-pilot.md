# Signposting pilot: local warm delivery

**Date:** 2026-08-27  
**Models:** Codex CLI 0.147.0 and Claude Code 2.1.247  
**Budget:** 150 ms, unchanged from the production contract  
**Truth set:** seven blinded, precommitted path/line acceptance sets

## Result

A resident, repo-local Bobbin index made signposting fast enough to deliver
inside the production budget. In the final affected-cell rerun (seven tasks ×
two families × `always-signpost`/`gated-signpost`), all 28 assignments completed
with zero failed artifacts and exactly seven rows in each of four cells.

The hooks observed 55 eligible semantic events. Six produced model-visible
signposts, with successful-delivery p95 **69 ms**. Three of 28 assignments saw
at least one signpost. No signpost was adopted. The remaining eligible calls
timed out or returned no candidate, so all-eligible p95 remained at the 150 ms
drop boundary. The delivery-path gate is satisfied; delivery coverage and
efficacy are not.

## What changed

- `PreToolUse` can launch detached, repo-scoped semantic work and publish an
  atomic cache entry.
- `PostToolUse` consumes a completed cache entry or uses its bounded context for
  a direct request to an already-warm local Bobbin server.
- `DP_SIGNPOST_PREFETCH=0` disables redundant speculation when the resident
  local server is itself the warm path.
- The original literal command is still never wrapped or modified. Cold start,
  malformed input, server error, and deadline expiry remain silent baseline.
- Correctness is scored from `eval/acceptance-sets.jsonl`, not exact strings
  embedded in model prompts. Missing or duplicate acceptance sets refuse.
- Campaign scoring now fails closed; a parse failure cannot increment the
  completion count without appending a valid row.

## Reproducibility and composition

The final 70-row result combines the 28 freshly remeasured affected cells with
42 baseline/prompt/replacement rows from the same pinned-corpus, blinded-set
campaign. Those six cells do not install the changed signposting hook. The
composite has 70 unique assignment IDs and exactly seven rows in every
model-family/condition cell.

The local index covered the five pinned corpora. Bobbin's installed ONNX loader
needed the host's existing versioned runtime path; after model startup was
amortized by a resident HTTP process, a separate 20-query probe measured 17.3 ms
p95. The campaign result above is stronger because it includes real hook
parsing, model execution, and model-visible context.

## Interpretation

This is a delivery result, not an efficacy result. The composite blinded scorer
marked 53 of 70 final locations correct, but the intervention produced zero
adoptions and zero signpost-correct assignments. No efficacy preprint should be
written from these pilots. The negative cold-path result and this local-path
delivery result are methods material for later work.

Artifacts:

- `eval/acceptance-sets.jsonl`
- `eval/results/2026-08-27-local-warm-results.jsonl`
- `eval/results/2026-08-27-local-warm-summary.json`
- `eval/run-campaign.sh`
