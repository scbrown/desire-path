# Quipu failure review

Status: implementation; capture is opt-in with `DP_QUIPU_REVIEW=1`.

Append a sanitized `quipu-review` invocation alongside the unchanged ordinary
invocation. Its `review_of` metadata links to the ordinary invocation ID. The
ordinary tool name, session, working directory, turn enrichment and failure
tracking retain their existing behavior. Privacy guarantees apply to the review
companion: it stores only a structural input pattern and categorical error, never
raw request or response prose. Existing ordinary records retain their original
content; enabling review does not redact historical or ordinary capture. Keep a session identifier and agent attribution with an explicit
unknown value. A successful transport carrying `isError` or `conforms:false` is
still a failure. An empty query is not a failure unless the caller explicitly
sets `expect_nonempty`. Missing response evidence is unknown, not success.

The normalized input pattern plus operation and error class identify a review
pattern. A scheduled consumer of `dp export --type invocations` files one review
issue per pattern, adds occurrence IDs to that issue, and emits a weekly digest.
Keep this issue-tracker adapter outside the portable collector. Never invoke an
issue tracker from the tool hook. Exact retry evidence requires a strictly later successful call in the same
observed session with a matching session-keyed canonical-input fingerprint.
Matching normalized shapes alone is only a candidate. Unknown sessions cannot establish an exact retry. Shell HTTP calls never claim
an exact retry: URL and header semantics are not fully observable. Even an exact successful retry does
not prove that a product defect was fixed. Fingerprints contain no raw inputs.

HTTP detection needs configured `DP_QUIPU_HTTP_HOSTS` (comma-separated hostnames)
or `QUIPU_SERVER`. It is observation only: no command is executed or retried.
Only a first literal `curl` command with supported options is recognized. Inline
JSON bodies and fixed methods contribute structural input patterns; file bodies
are never read. Shell exit status alone is not HTTP failure evidence: a downstream
zero-match filter can exit nonzero after a successful request. Observed response
JSON, explicit HTTP status (including 408 as timeout), or curl diagnostic evidence
is required. An optional stdout trailer `DP_QUIPU_HTTP_STATUS=NNN` supplies status
evidence when ordinary capture retains it.

Calls made inside opaque scripts, suppressed responses, or detached processes
are not observable from a tool completion alone. Report that coverage gap; do
not call zero observed failures a healthy service. A valid but unhelpful search
also requires an explicit expectation or user feedback.

Validation: isolated installed-binary baseline, envelope/semantic-negative tests,
credential-bearing fixtures with negative persistence assertions, and a repeated
baseline against the candidate. Fleet activation and scheduled bug/digest
reporting require separately reviewed configuration and observed scheduled runs.

Resource limits: input JSON64KiB, response JSON1MiB, normalized patterns8KiB,
object width32, array width8, nesting8. Exceeded bounds retain unknown/redacted
structure rather than raw text. Explicit failures override successful wrappers.

Real-envelope control: `testdata/owned-claude-hook.json` was emitted by Claude Code
2.1.295 through an isolated explicit PostToolUse command hook after a constant
read-only MCP query returned control value 1. Tool name and input are unchanged;
session/path identifiers are redacted. The separate content-block fixture records
response topology only, with reconstructed input, and is not real-hook acceptance.
Neither fixture establishes fleet activation or consumer adoption.
