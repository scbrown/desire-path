# Quipu failure review

Status: implementation; capture is opt-in with `DP_QUIPU_REVIEW=1`.

Normalize observed MCP and configured HTTP calls during ingest, before persistence.
Store only a structural input pattern and categorical error, never raw request or
response prose. Keep a session identifier and agent attribution with an explicit
unknown value. A successful transport carrying `isError` or `conforms:false` is
still a failure. An empty query is not a failure unless the caller explicitly
sets `expect_nonempty`. Missing response evidence is unknown, not success.

The normalized input pattern plus operation and error class identify a review
pattern. A scheduled consumer of `dp export --type invocations` files one review
issue per pattern, adds occurrence IDs to that issue, and emits a weekly digest.
Keep this issue-tracker adapter outside the portable collector. Never invoke an
issue tracker from the tool hook. Exact retry evidence requires a strictly later successful call in the same
observed session with a matching session-keyed canonical-input fingerprint.
Matching normalized shapes alone is only a candidate. Unknown sessions or opaque
HTTP inputs cannot establish an exact retry. Even an exact successful retry does
not prove that a product defect was fixed. Fingerprints contain no raw inputs.

HTTP detection needs configured `DP_QUIPU_HTTP_HOSTS` (comma-separated hostnames)
or `QUIPU_SERVER`. It is observation only: no command is executed or retried.
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
