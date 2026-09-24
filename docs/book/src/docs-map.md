# Docs map

The [user guide](introduction.md) is built from `docs/book`. Start with
[getting started](getting-started.md), [agent setup](integrations/README.md),
[commands](commands/README.md), or [configuration](configuration.md).

## Supporting documents

Design notes record intent; the released CLI and current reference describe available behavior.

- [`assets/banner-dark.svg`](https://github.com/scbrown/desire-path/blob/main/docs/assets/banner-dark.svg) — README banner for dark backgrounds.
- [`assets/banner-light.svg`](https://github.com/scbrown/desire-path/blob/main/docs/assets/banner-light.svg) — README banner for light backgrounds.
- [`design-codex-source-plugin.md`](https://github.com/scbrown/desire-path/blob/main/docs/design-codex-source-plugin.md) — Codex event and installer design; compare with the installed source plugin.
- [`plans/001-initial-plan.md`](https://github.com/scbrown/desire-path/blob/main/docs/plans/001-initial-plan.md) — **Historical** — original architecture and initial implementation plan.
- [`plans/002-invocation-tracking.md`](https://github.com/scbrown/desire-path/blob/main/docs/plans/002-invocation-tracking.md) — **Historical** — design for tracking successes as well as failures.
- [`plans/003-kiro-source-plugin.md`](https://github.com/scbrown/desire-path/blob/main/docs/plans/003-kiro-source-plugin.md) — **Historical** — Kiro source-plugin implementation plan.
- [`plans/004-cursor-source-plugin.md`](https://github.com/scbrown/desire-path/blob/main/docs/plans/004-cursor-source-plugin.md) — **Historical** — Cursor source-plugin implementation plan.
- [`plans/004-gemini-cli-source-plugin.md`](https://github.com/scbrown/desire-path/blob/main/docs/plans/004-gemini-cli-source-plugin.md) — Proposal for Gemini CLI capture; not a claim of released support.
- [`plans/006-pave-command-corrections.md`](https://github.com/scbrown/desire-path/blob/main/docs/plans/006-pave-command-corrections.md) — Design for flag-aware command correction.
- [`plans/007-recipe-aliases.md`](https://github.com/scbrown/desire-path/blob/main/docs/plans/007-recipe-aliases.md) — Proposal for multi-step recipe expansion.
- [`plans/008-turn-reconstruction.md`](https://github.com/scbrown/desire-path/blob/main/docs/plans/008-turn-reconstruction.md) — Design for reconstructing turns from transcripts.
- [`plans/009-new-dp-suggest.md`](https://github.com/scbrown/desire-path/blob/main/docs/plans/009-new-dp-suggest.md) — Design for suggestions from observed failures.
- [`plans/009-signposting-eval.md`](https://github.com/scbrown/desire-path/blob/main/docs/plans/009-signposting-eval.md) — Signposting contract, event definitions, and evaluation design.
- [`plans/backlog.md`](https://github.com/scbrown/desire-path/blob/main/docs/plans/backlog.md) — Candidate future work; not the released feature list.
- [`private-key-checks.md`](https://github.com/scbrown/desire-path/blob/main/docs/private-key-checks.md) — Contributor guide to the private-key refusal controls.
- [`tasks/phase-1-core.md`](https://github.com/scbrown/desire-path/blob/main/docs/tasks/phase-1-core.md) — **Historical** — Phase 1: Core (Collect + Store) implementation checklist.
- [`tasks/phase-2-reporting.md`](https://github.com/scbrown/desire-path/blob/main/docs/tasks/phase-2-reporting.md) — **Historical** — Phase 2: Reporting implementation checklist.
- [`tasks/phase-3-suggestions.md`](https://github.com/scbrown/desire-path/blob/main/docs/tasks/phase-3-suggestions.md) — **Historical** — Phase 3: Suggestions & Aliases implementation checklist.
- [`tasks/phase-4-polish.md`](https://github.com/scbrown/desire-path/blob/main/docs/tasks/phase-4-polish.md) — **Historical** — Phase 4: Polish implementation checklist.
- [`tasks/phase-5-invocations.md`](https://github.com/scbrown/desire-path/blob/main/docs/tasks/phase-5-invocations.md) — **Historical** — Phase 5: Invocation Tracking implementation checklist.

## Supplementary development book

The root [`book/`](https://github.com/scbrown/desire-path/blob/main/book/src/README.md) contains supplementary architecture,
operations, and development notes. It is a separate tree and is not the book
published by the Pages workflow. Use the current user guide above for CLI setup.

## Theme provenance

The Coal override in `docs/book/custom/css/custom.css` is vendored byte-for-byte
from [Caboodle at b351e6de](https://github.com/scbrown/caboodle/blob/b351e6debd4e9b6f5e461d9ebbfa264feb9f4363/docs/book/custom/css/custom.css).
Keep it in sync with the [stack standard](https://github.com/scbrown/caboodle/blob/main/docs/stack/README-STANDARD.md).
