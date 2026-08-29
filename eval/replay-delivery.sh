#!/usr/bin/env bash
# Delivery-coverage replay. NO MODEL IN THE LOOP, and therefore NO PROVIDER QUOTA.
#
# Signpost delivery is a property of the hook and the semantic index, not of the
# model: whether a signpost reaches the model's context is decided before the
# model ever sees it. So coverage can be measured, and a campaign's achievable n
# of model-visible signposts can be certified, without spending a single
# assignment. The pilot spent 70 assignments to discover that zero signposts had
# been delivered; this harness answers that question for free.
#
# Each row of the workload is replayed as a PostToolUse payload carrying the
# recorded command and a response with the recorded line count, so the gating
# predicate is reproduced exactly. With --prefetch-gap-ms N the PreToolUse
# prefetch hook runs first and the replay waits N ms before the PostToolUse
# hook, standing in for the time the model's own literal search takes.
#
# usage:
#   eval/replay-delivery.sh --dp PATH --bobbin-server URL --out DIR \
#       [--workload FILE] [--condition always-signpost] \
#       [--timeout-ms 150] [--prefetch-gap-ms N | --no-prefetch] [--limit N]
set -euo pipefail

usage() { sed -n '2,20p' "$0"; exit "${1:-2}"; }

dp_bin=; bobbin_server=; out=; workload=eval/replay-workload.jsonl
condition=always-signpost; timeout_ms=150; gap_ms=-1; limit=0
while (($#)); do
	case $1 in
	--dp) dp_bin=$2; shift 2 ;;
	--bobbin-server) bobbin_server=$2; shift 2 ;;
	--out) out=$2; shift 2 ;;
	--workload) workload=$2; shift 2 ;;
	--condition) condition=$2; shift 2 ;;
	--timeout-ms) timeout_ms=$2; shift 2 ;;
	--prefetch-gap-ms) gap_ms=$2; shift 2 ;;
	--no-prefetch) gap_ms=-1; shift ;;
	--limit) limit=$2; shift 2 ;;
	-h | --help) usage 0 ;;
	*) usage >&2 ;;
	esac
done
for required in dp_bin bobbin_server out; do
	[[ -n ${!required} ]] || { printf 'missing --%s\n' "${required//_/-}" >&2; exit 2; }
done
[[ -r $workload ]] || { printf 'no workload: %s\n' "$workload" >&2; exit 2; }
command -v jq >/dev/null || { printf 'jq is required\n' >&2; exit 2; }

# Fail closed on an unreachable backend. A backend that is simply down would
# otherwise render as a real coverage measurement of zero, which is the exact
# shape of mistake this whole workstream keeps paying for.
if ! curl -sf -m 10 --get --data-urlencode 'q=probe' --data 'limit=1' "$bobbin_server" >/dev/null; then
	printf 'bobbin backend did not answer a control query: %s\n' "$bobbin_server" >&2
	exit 3
fi

mkdir -p "$out"
arm=direct
((gap_ms >= 0)) && arm=prefetch-${gap_ms}ms
events=$out/events-$arm.jsonl
: >"$events"
cache=$out/cache-$arm
rm -rf "$cache"
mkdir -p "$cache"

n=0
while IFS= read -r row; do
	((limit > 0 && n >= limit)) && break
	n=$((n + 1))
	command=$(jq -r .command <<<"$row")
	cardinality=$(jq -r .result_cardinality <<<"$row")
	repo=$(jq -r .repo <<<"$row")
	# A response with exactly the recorded number of lines: the hook's predicate
	# is a line count, so this reproduces the gate without re-running the search
	# against a corpus that may have moved.
	response=$(awk -v n="$cardinality" 'BEGIN { for (i = 0; i < n; i++) print "line" i }')
	payload=$(jq -cn --arg c "$command" --arg r "$response" --arg id "replay-$n" \
		'{session_id:"replay",tool_use_id:$id,tool_name:"Bash",tool_input:{command:$c},tool_response:$r,cwd:"/replay"}')

	if ((gap_ms >= 0)); then
		env DP_SIGNPOST_REPO="$repo" DP_SIGNPOST_BOBBIN_URL="$bobbin_server" \
			DP_SIGNPOST_CACHE_DIR="$cache" "$dp_bin" signpost-prefetch <<<"$payload" >/dev/null 2>&1 || true
		sleep "$(awk -v ms="$gap_ms" 'BEGIN { printf "%.3f", ms / 1000 }')"
	fi
	env DP_SIGNPOST_CONDITION="$condition" DP_SIGNPOST_REPO="$repo" \
		DP_SIGNPOST_LOG="$events" DP_SIGNPOST_BOBBIN_URL="$bobbin_server" \
		DP_SIGNPOST_TIMEOUT_MS="$timeout_ms" DP_SIGNPOST_CACHE_DIR="$cache" \
		DP_SIGNPOST_PREFETCH=$((gap_ms >= 0 ? 1 : 0)) \
		"$dp_bin" signpost <<<"$payload" >/dev/null 2>&1 || true
done <"$workload"

summary=$out/coverage-$arm.json
jq -s --arg arm "$arm" --arg condition "$condition" --argjson replayed "$n" \
	--argjson timeout_ms "$timeout_ms" --argjson gap_ms "$gap_ms" '
	{ arm: $arm, condition: $condition, replayed: $replayed,
	  timeout_ms: $timeout_ms, prefetch_gap_ms: (if $gap_ms < 0 then null else $gap_ms end),
	  eligible: length,
	  shown: (map(select(.signpost_shown)) | length),
	  warm_cache_hits: (map(select(.warm_cache_hit)) | length),
	  timed_out: (map(select((.signpost_shown | not) and .semantic_latency_ms >= ($timeout_ms - 10))) | length),
	  zero_candidate: (map(select((.signpost_shown | not) and .semantic_latency_ms < ($timeout_ms - 10))) | length),
	  coverage: (if length == 0 then null else ((map(select(.signpost_shown)) | length) / length) end),
	  shown_latency_ms: (map(select(.signpost_shown) | .semantic_latency_ms) | sort) }
	' "$events" >"$summary"
jq -c '{arm,eligible,shown,coverage,timed_out,zero_candidate,warm_cache_hits}' "$summary"
printf 'events: %s\nsummary: %s\n' "$events" "$summary" >&2
