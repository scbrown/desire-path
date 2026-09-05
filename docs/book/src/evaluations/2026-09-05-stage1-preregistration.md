# Stage 1 preregistration — signposting at scale, plus a payload arm

**Filed:** 2026-09-05, before the first assignment ran.
**Envelope:** 200 assignments (main matrix) + 40 (payload arm), authorised on
aegis-vkirh and scoped on aegis-3npm7w.
**Gate cleared:** Stage 0 certified delivery coverage at 100% against the 25%
gate (`2026-08-29-stage0-delivery-coverage.md`).

This document is committed **before** the campaign so the analysis cannot be
chosen after seeing the numbers. Everything below is fixed at filing time.

## Why there are two matrices and not one

The two pilots agree on one thing: agents do not take pointers. Across both,
zero signposts were adopted, and the second one established that the instrument
— not the intervention — had been the problem.

So this campaign asks two different questions, and keeps them apart:

| matrix | arms | question |
|---|---|---|
| **main** | `bare-literal`, `prompt-semantic`, `replacement`, `always-signpost`, `gated-signpost` | does being *pointed at* the stack change anything? |
| **payload** | `payload-signpost` | does being *handed the answer* change anything? |

**They are planned separately and scored separately, and their adoption rates
must never be pooled**, because adoption is not the same act in the two:

- a pointer arm's signpost names a command, so **adoption = the agent ran it**;
- the payload arm has already run it, so **adoption = the agent used a location
  the payload handed over**.

Both facts are recorded on every row (`ran_stack_command`,
`used_injected_location`) and `adoption_kind` says which one `adopted` holds.
`dp eval score` refuses to aggregate a cell whose rows disagree about the kind.
The metric that IS directly comparable across every arm is **correctness**, and
it is the one to read first when the two adoption numbers differ.

## Fixed design

- **Tasks:** 20, each with a blinded acceptance set adjudicated and committed
  before the campaign (`eval/tasks.jsonl`, `eval/acceptance-sets.jsonl`).
  `eval/check-acceptance.sh` asserts, at the pinned revisions, that every
  ground truth exists and falls inside its own accepted range — the defect that
  made the 2026-08-27 pilot's correctness numbers unpublishable was an
  acceptance set that excluded valid nearby locations, so this is now a gate and
  not a habit.
- **Families:** codex and claude. Two families is a stated **generality limit**,
  not a claim about models in general.
- **Conditions:** the five above; 20 x 2 x 5 = **200**. Payload arm:
  20 x 2 x 1 = **40**.
- **Replicates:** 1. The blocking is over tasks, not repeats.
- **Assignment:** deterministic blocked matrix, seed `stage1`, shuffled by hash
  so no arm runs in a favourable block.
- **Budget:** the 150 ms PostToolUse contract, unchanged. The PreToolUse
  prefetch runs, as it does in production.
- **Index:** the local resident index over the five pinned corpora
  (`eval/build-local-index.sh`). Certification runs against this, not against a
  shared backend that does not hold every corpus.

## Endpoints, fixed now

- **Primary:** adoption rate among model-visible signposts, per family x
  condition, with `adoption_kind` attached.
- **Secondary:** correctness rate, turns-to-locate, tokens-to-resolution, all
  per family x condition.
- **Payload-arm comparison:** `payload-signpost` against `always-signpost` —
  they apply the same gate and differ only in what is injected. It is not
  compared against `gated-signpost`.
- **Intent families** are recorded per row (`intent_families`) and reported, but
  the campaign is **not** powered to compare families against each other; that
  is delivery-harness work, below.

## The failure reading, stated in advance

An adoption rate indistinguishable from zero at n >= 60 model-visible signposts
is a **publishable negative result** about signposting as an intervention. It
closes the question; it does not motivate a third campaign.

If the payload arm is adopted where the pointer arm is not, the finding is about
**pointer vs payload**, not about signposting being fixed — the payload arm
spends stack work on every eligible search whether or not it is wanted, and that
cost is the next thing to measure, not this campaign's result.

## No post-hoc arm selection

The arms, the endpoints, the comparator, and the failure reading are the ones in
this file. Anything discovered afterwards is reported as exploratory and labelled
as such.

## What is certified on the delivery harness instead of here

Intent widening (`find -name`, `git log -S|--grep`, and symbol promotion) is a
**delivery** property and is measured with no model in the loop, on the replay
harness, paired and interleaved against the pre-change build. Model quota is not
spent on it. See the Stage 1 delivery note.
