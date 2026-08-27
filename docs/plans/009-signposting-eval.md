# Signposting evaluation and event contract

Status: implementation contract for the private signposting sibling mode.

## Invariants

Signposting runs only after the deterministic search has completed. It never
executes, wraps, shadows, or rewrites that search. Therefore the search's stdout
and exit status remain byte-identical to an installation without signposting.
Hook failure, timeout, malformed input, and Bobbin failure all produce no hook
output and exit successfully: worst case equals the baseline.

V1 observes Bash-mediated `grep` and `rg` commands through a `PostToolUse` hook.
Claude Code's native Grep and Glob tools are explicitly outside the experiment.

## Conditions

The evaluation harness assigns one of five conditions before a task begins:

1. `bare-literal`
2. `prompt-semantic`
3. `replacement`
4. `always-signpost`
5. `gated-signpost`

The production default is `gated-signpost`. Its predicates are `null` (zero
result lines) and `high-cardinality` (more than the configured threshold).

## Task records

Ground-truth task fixtures are JSONL with these required fields:

```json
{"task_id":"repo-001","repo":"owner/repo","revision":"git-sha","query":"where is retry backoff handled","class":"weak","ground_truth":["path:line"],"model_family":"family","condition":"gated-signpost"}
```

`class` is one of `strong`, `weak`, or `misleading`. Revisions are immutable.
Ground truth is adjudicated before any model run.

## Signpost event records

The hook appends one JSON object per eligible literal search. The schema is
defined before capture code so one campaign can calculate every planned metric:

| Field | Meaning |
|---|---|
| `event_id`, `timestamp`, `session_id` | join/provenance keys |
| `task_id`, `model_family`, `condition` | experiment assignment; may be empty outside the harness |
| `tool`, `command`, `cwd`, `query` | observed literal-search context |
| `result_cardinality`, `threshold`, `predicate` | why the signpost was or was not eligible |
| `signpost_shown`, `semantic_candidate_count`, `semantic_latency_ms` | intervention and cost |
| `adopted`, `turns_to_locate`, `tokens_to_resolution`, `correct` | nullable outcomes filled by the evaluator |

`predicate` is `none`, `null`, or `high-cardinality`. A timeout records an
event with `signpost_shown=false` only when the logger itself remains within the
budget; logging is never allowed to delay hook completion.

## Metrics and gates

- Adoption rate: adopted / shown.
- Turns and tokens to resolution: compare distributions by condition and task.
- Correctness rate: ground-truth hit before the first write.
- Signpost precision: shown signposts whose followed semantic result reaches a
  ground-truth location.
- Latency: p50/p95 semantic latency and total hook wall time.

No adoption claim is reported until signpost precision is measured on tasks
whose semantic query has zero literal-term overlap with the ground-truth symbol.
Every condition must run the same revision/task/model-family blocks.

## Reproducible campaign plan

Pinned corpora live in `eval/corpora.json`; adjudicated tasks live in
`eval/tasks.jsonl`. Generate the complete blocked matrix with:

```bash
dp eval plan --tasks eval/tasks.jsonl \
  --models codex,claude,gemini --replicates 1 --seed signposting-v1 \
  > assignments.jsonl
```

The planner refuses non-SHA revisions, empty ground truth, unknown task classes,
duplicate task IDs, fewer than three model families, and invalid replicate
counts. It does not invoke models: execution and outcome adjudication are
separate, auditable steps, so a generated plan cannot be mistaken for results.

After adjudication, `dp eval score --results results.jsonl` groups outcomes by
model family and condition and reports adoption rate, correctness rate,
signpost precision, and average turns/tokens to resolution. Empty or malformed
result sets are refused rather than rendered as zero-effect findings.
