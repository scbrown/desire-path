# Stage 1 results: agents will not take a pointer, will take an answer, and are no more correct either way

**Date:** 2026-09-05
**Preregistered:** `2026-09-05-stage1-preregistration.md`, committed before the
first assignment ran.
**Spent:** 240 assignments (200 main matrix + 40 payload arm).

## Integrity

| | main | payload |
|---|--:|--:|
| rows / distinct assignment ids | 200 / 200 | 40 / 40 |
| rows per cell | 20 in each of 10 | 20 in each of 2 |
| permission denials | **0** | **0** |
| scoring failures | **0** | **0** |

An earlier run of the main matrix was discarded and re-run: 38 of its 189
completed rows and all 11 of its failures carried a Bash permission denial, all
49 in one family. That is recorded in full below, because it nearly became the
result.

## The primary endpoint

**Pointer arms: 53 model-visible signposts, ZERO adopted.** Both families.
0 of 53 bounds adoption at **5.7%** (95%, rule of three).

**Payload arm: 26 of 40 adopted — 65%**, and 65% in *each* family independently.

The two adoption metrics are different acts and are not pooled: a pointer is
adopted by RUNNING the command it names, a payload by USING a location it
already fetched. Every row carries `adoption_kind`, and the scorer refuses to
aggregate a cell whose rows disagree about it. What the two numbers jointly
support is narrow and solid: **the same agents that ignore a command will use an
answer.**

## The endpoint that is directly comparable says nothing changed

| arm | n | adopted | correct | correct in 1 search | turns | tokens |
|---|--:|--:|--:|--:|--:|--:|
| payload-signpost | 40 | 26 | **34** | 6 | 3.83 | 145k |
| always-signpost | 40 | 0 | **33** | 7 | 4.33 | 160k |
| bare-literal (baseline) | 40 | 0 | **33** | 6 | 4.22 | 160k |

**One task.** Correctness is 34/40 against 33/40 against 33/40. Turns fall by
half a step and tokens by ~9%, both well inside noise at n=40.

Across the whole main matrix correctness is flat against each family's own
baseline — claude 0.80 vs 0.80, codex 0.85 vs 0.85 — and so are turns and tokens.

## The positive control that makes this a result and not a dead instrument

The same 200 rows separate task difficulty cleanly:

    strong 0.98      weak 0.79      misleading 0.72

The instrument detects a large difference in exactly this data. It detects none
between conditions. That is the difference between "we found nothing" and "there
is nothing here".

## Reading, exactly as preregistered

The preregistration fixed this reading in advance, and it is the one that
applies:

> If the payload arm is adopted where the pointer arm is not, the finding is
> about **pointer vs payload**, not about signposting being fixed — the payload
> arm spends stack work on every eligible search whether or not it is wanted,
> and that cost is the next thing to measure, not this campaign's result.

So: signposting-as-a-pointer is a **publishable negative result**. Adoption is
bounded at 5.7% and correctness is unmoved. It closes that question rather than
motivating a third campaign.

Signposting-as-a-payload is **adopted and inert**. It fixes the adoption problem
completely and buys no measurable correctness. Before it ships anywhere it has to
justify a cost the pointer arm does not pay: a stack query on every eligible
search, delivered whether or not it was wanted.

## Exploratory, and confounded — stated so it is not mistaken for a finding

Within the payload arm, rows where the agent used an injected location were
correct 24/26; rows where it did not were correct 10/14. This is **not** evidence
that adoption causes correctness. Agents plausibly adopt precisely when the
payload is good, which is when retrieval succeeded, which is when the task was
easier. The comparison is post-hoc, the arms are self-selected, and it is
reported only because omitting it would be selective.

## What nearly became the result instead

`--permission-mode dontAsk` with `--setting-sources ''` loads no permission
rules, so Claude denied Bash calls it did not auto-approve. An agent that cannot
run its search still produces a scored row, and that row reads as a failure of
the intervention.

    38 of 189 completed runs   carried "denied because Claude Code is
    11 of 11 that failed to score       running in don't ask mode"
    49 of 49 were claude.  0 were codex.

Entirely in one family, condition-correlated, and shaped exactly like "signposting
does not help" — high turns, high tokens, low correctness. The baselines had been
given no settings file at all, since one was written only to carry hooks, so the
arms differed in more than the intervention.

Permissions are part of the instrument. Every claude condition now gets a
settings file carrying the Bash allow, with hooks added only where the arm calls
for them. The contaminated rows were deleted and re-run rather than analysed; the
151 clean rows were kept, because nothing was denied in them.

## Decisions taken on this evidence

Both are deliberate choices with reasons, not defaults that happened.

**Payload mode is NOT rolled out to the fleet.** It is adopted and inert: 65%
uptake buying one task out of forty. Rolling it out would pay a stack query on
every eligible search, on every pane, forever, for a return this campaign
measured as noise. Production stays on the pointer mode already installed —
cheap, delivered, and never taken. Recording that as a decision matters: "we left
it on pointers" would otherwise read as inertia, when it is a choice made against
a number.

**Widening the intent families further is dropped.** Two families beyond the
original were built and certified for delivery — `symbol-search`, which is 18% of
recorded searches and answers at p50 11 ms against 79-93 ms, and `file-find` at
15/15 — and a third, `history-search`, is implemented but uncertified.

Certifying it would be work in service of a hypothesis this campaign has already
answered. **Widening changes WHICH searches are eligible for a signpost; it does
not change what an agent does with one.** The pointer arm was adopted 0 times out
of 53 across three families, and the payload arm's 65% uptake moved correctness
by a single task. A fourth family multiplies the denominator of an intervention
whose numerator is the problem. That is why it is dropped rather than deferred:
deferring implies the evidence might change, and the evidence that would change
it is not a family count.

What the family work does leave behind is worth keeping: the structural route is
eight times faster than semantic search on the same query, which is a fact about
the stack rather than about signposting, and it is measured and recorded.

## Limits, stated rather than found later

- **n = 53 model-visible signposts, against a preregistered target of 60.** The
  estimate assumed more eligible searches per assignment than agents make.
  53 still bounds adoption below 5.7%, and the preregistered minimum was 30, but
  the shortfall is real.
- **Two model families.** A stated generality limit, not a claim about models.
- **The payload arm's adoption metric can be satisfied by coincidence** — an
  agent could reach an injected path by its own search. `resolved_in_one_search`
  was recorded as a non-inferential cross-check and shows no difference (6 vs 7),
  which is itself part of why the correctness reading is "nothing changed".
- **One task set, five corpora, one index.**
