# Prospective payload adoption

The current signpost log describes offers, not subsequent file use. The installed
binary drops non-search/non-Bash events; generic ingest drops tool input. Reading
transcripts after the experiment cannot repair the missing observation.

The fixed primary measure is same-session Read/Edit/open of an offered path
within the next ten tool invocations, compared with a different randomly paired
payload event. Run seven days from an observed logger installation, require at
least 300 payload events, and report the previously chosen 2x success and 1.2x
null thresholds. Do not automatically change payload delivery from this logger.

## Collector

Use a separate private SQLite store, enabled only by
`DP_SIGNPOST_ADOPTION_DIR`. Default-off does not parse input or open a store.
PreToolUse assigns a session-local invocation ordinal to an exact call ID.
PostToolUse/Failure completes that call and records explicit file operations.
All tools consume ordinals; unrelated sessions and duplicate hook deliveries do
not consume an offer's window. Register an offer immediately before signpost
emission, with the current ordinal as its boundary. Calls already started before
that registration are excluded, even if they complete later. A missing pre-call
or incomplete window is unknown, never a negative outcome.

Choose a baseline uniformly from previous, different payload events at offer
registration; use a bounded reservoir sampled uniformly from all earlier offers.
The first event has no donor and is reported as unpaired. Store both outcomes at
hook time; report incomplete/censored and invalid rows separately. No verdict on
missing observations or an undefined zero-base ratio.

Store session/call/path identities as keyed hashes in the private store. Never
persist raw commands, responses, transcripts, CWDs or file contents there.
Bound input, paths, sessions, pending windows and donor state. Serialize each
transition with SQLite's writer transaction and dedupe by exact identifiers.
Failure to observe must not change signpost delivery, invocation records, or the
original event JSONL. Report coverage against that original log, tolerantly
counting malformed lines.

## Path identity

Keep the search response's repository alongside each emitted path internally;
never guess that an unscoped relative result belongs to the caller's CWD.
`DP_SIGNPOST_ADOPTION_ROOTS` maps index repository names to local checkouts.
Normalize actual file operations through repository roots/origins so linked
worktrees and independent clones match the same source file, while an identical
relative filename in a different repository does not match. Unaddressable offers
remain unknown. Existing payload text and the original JSONL schema stay intact.

Count explicit Read/Edit/Write, patch file headers, file-opening MCP operations,
and supported literal shell file operations. This is a tool path-use proxy,
not proof of comprehension, utility, or a causal effect of injection. Document
unsupported/dynamic operations rather than claiming complete filesystem tracing.

## Arming and evaluation

A tier-2 review is required before merging/arming fleet hooks or persistence.
Use the normal release and hook-bundle provisioning path. Verify a real all-tool
probe (including unrelated tool calls), then record the observed install time.
No clock starts from a source commit, shell-only test, or an unobserved install.
Keep malformed counts, source/harness coverage, missing offers and unresolved
path identities in the final seven-day report. Prospective data only; never
replace the retired explicit-Bobbin-share metric or reinterpret its old window.

A dropped observer/offer write also leaves a sticky private filesystem gap marker,
independent of the SQLite writer lock. Reports suppress rates when this marker is
present. Starting a new collector directory is required for a clean prospective
experiment; deleting the marker is not a recovery procedure. Compound shell
commands and failed tool invocations are unknown rather than positive path use.

Unsupported shell commands and unrecognized file-tool names are also unknown;
a small literal non-file shell allowlist can be confidently negative. This may
leave many real windows unscorable. Measure that native coverage before starting
the seven-day clock; insufficient coverage is a feasibility failure, never the
null result that would retire injection. Do not silently restrict the denominator
to easy windows and report it as all payload events.
