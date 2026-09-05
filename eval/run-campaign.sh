#!/usr/bin/env bash
set -u

usage() {
	printf '%s\n' 'usage: eval/run-campaign.sh --assignments FILE --acceptance-sets FILE --corpus-root DIR --artifacts DIR --dp PATH --bobbin-server BASE_URL [--signpost-timeout-ms N] [--limit N]'
	printf '%s\n' '--bobbin-server is the BASE url (no /search): the hook route is derived from it, and the agent bobbin is pointed at it directly.'
}

assignments=
acceptance_sets=
corpus_root=
artifacts=
dp_bin=
bobbin_server=
limit=0
signpost_timeout_ms=150
while (($#)); do
	case "$1" in
		--assignments) assignments=$2; shift 2 ;;
		--acceptance-sets) acceptance_sets=$2; shift 2 ;;
		--corpus-root) corpus_root=$2; shift 2 ;;
		--artifacts) artifacts=$2; shift 2 ;;
		--dp) dp_bin=$2; shift 2 ;;
		--bobbin-server) bobbin_server=$2; shift 2 ;;
		--signpost-timeout-ms) signpost_timeout_ms=$2; shift 2 ;;
		--limit) limit=$2; shift 2 ;;
		-h|--help) usage; exit 0 ;;
		*) usage >&2; exit 2 ;;
	esac
done
for required in assignments acceptance_sets corpus_root artifacts dp_bin bobbin_server; do
	if [[ -z ${!required} ]]; then
		printf 'missing --%s\n' "${required//_/-}" >&2
		exit 2
	fi
done
for command in jq codex claude bobbin; do
	command -v "$command" >/dev/null || { printf 'missing command: %s\n' "$command" >&2; exit 2; }
done
[[ -x $dp_bin ]] || { printf 'dp is not executable: %s\n' "$dp_bin" >&2; exit 2; }

# PREFLIGHT, FAIL CLOSED. A corpus the backend does not index answers every
# query with zero, and an agent that ADOPTS a signpost into an empty index gets
# nothing back -- which scores as "the intervention does not help" when what was
# actually measured is a backend that cannot see the repo. Stage 0 found exactly
# this: the shared fleet backend holds 3 of the 5 pinned corpora. So every
# corpus is asked a real question before any quota is spent, and a misspelled
# repo is asked too, because a backend that answers EVERYTHING is not a control.
preflight_backend() {
	local failed=0 name count
	while IFS= read -r name; do
		count=$(curl -sf -m 20 --get --data-urlencode 'q=error handling' --data 'limit=1' \
			--data "repo=$name" "$bobbin_server/search" | jq -r '.count // 0') || count=unreachable
		printf 'preflight %-14s %s\n' "$name" "$count" >&2
		[[ $count =~ ^[1-9] ]] || failed=1
	done < <(jq -r '.[].repo | split("/") | last' eval/corpora.json)
	count=$(curl -sf -m 20 --get --data-urlencode 'q=error handling' --data 'limit=1' \
		--data 'repo=not_a_real_corpus' "$bobbin_server/search" | jq -r '.count // 0') || count=unreachable
	printf 'preflight %-14s %s (negative control, must be 0)\n' "not_a_real_corpus" "$count" >&2
	[[ $count == 0 ]] || failed=1
	return $failed
}
preflight_backend || {
	printf 'backend cannot see every pinned corpus (or answered the negative control) -- not spending the envelope\n' >&2
	exit 3
}

mkdir -p "$artifacts/raw" "$artifacts/events" "$artifacts/settings" "$artifacts/replacement-bin"
lock=$artifacts/.running
if ! mkdir "$lock" 2>/dev/null; then
	printf 'campaign already running (or stale lock): %s\n' "$lock" >&2
	exit 4
fi
trap 'rmdir "$lock"' EXIT
replacement=$(cd "$(dirname "$0")" && pwd)/replacement-search
chmod +x "$replacement"
ln -sf "$replacement" "$artifacts/replacement-bin/grep"
ln -sf "$replacement" "$artifacts/replacement-bin/rg"
bobbin_bin=$(command -v bobbin)
results=$artifacts/results.jsonl
touch "$results"

prompt_for() {
	local query=$1 condition=$2
	local instruction='Begin with Bash-mediated grep or rg. Do not use native Grep/Glob tools or semantic search unless a signpost offers it.'
	case "$condition" in
		prompt-semantic) instruction='Prefer Bash-mediated `bobbin search --repo REPO QUERY` over grep or rg; use literal search only if needed.' ;;
		replacement) instruction='Use exactly one Bash-mediated `rg QUERY .` search first. The evaluation environment supplies that search implementation.' ;;
	esac
	printf 'Evaluation task. Locate the code described by: %s\nWork read-only at the pinned revision. %s Do not modify files. Finish with exactly one line: LOCATION path:line\n' "$query" "$instruction"
}

# score_one derives one result row. ADOPTION IS NOT ONE MEASUREMENT: for a
# pointer arm it means the agent RAN the stack command it was handed; for the
# payload arm the command was already run for it, so adoption means the agent
# USED a location the payload named. Both are recorded on every row, and
# `adoption_kind` says which one `adopted` holds, so nothing downstream can sum
# two different questions into one rate by accident.
score_one() {
	local assignment=$1 acceptance=$2 raw=$3 events=$4 model=$5 condition=$6
	local final turns tokens ran_stack shown correct signpost_correct
	local used_payload adopted adoption_kind families
	if [[ $model == codex ]]; then
		final=$(jq -rs '[.[] | select(.type=="item.completed" and .item.type=="agent_message") | .item.text] | last // ""' "$raw") || return
		turns=$(jq -rs '[.[] | select(.type=="item.completed" and .item.type=="command_execution")] | length' "$raw") || return
		tokens=$(jq -rs '[.[] | select(.type=="turn.completed") | (.usage.input_tokens + .usage.output_tokens)] | add // 0' "$raw") || return
		ran_stack=$(jq -rs 'any(.[]; .type=="item.completed" and .item.type=="command_execution" and ((.item.command // "") | test("(^|[[:space:]])(bobbin[[:space:]]+search|yupana[[:space:]]+(callers|impact))")))' "$raw") || return
	else
		final=$(jq -rs '[.[] | select(.type=="result") | .result] | last // ""' "$raw") || return
		turns=$(jq -rs '[.[] | select(.type=="assistant") | .message.content[]? | select(.type=="tool_use" and .name=="Bash")] | length' "$raw") || return
		tokens=$(jq -rs '[.[] | select(.type=="result") | (.usage.input_tokens + .usage.cache_creation_input_tokens + .usage.cache_read_input_tokens + .usage.output_tokens)] | add // 0' "$raw") || return
		ran_stack=$(jq -rs 'any(.[]; .type=="assistant" and any(.message.content[]?; .type=="tool_use" and .name=="Bash" and ((.input.command // "") | test("(^|[[:space:]])(bobbin[[:space:]]+search|yupana[[:space:]]+(callers|impact))"))))' "$raw") || return
	fi
	shown=false
	used_payload=false
	families='[]'
	if [[ -s $events ]]; then
		shown=$(jq -s 'any(.[]; .signpost_shown == true)' "$events")
		families=$(jq -sc '[.[] | select(.signpost_shown) | .intent_family] | unique' "$events")
		# Did the final answer land on a path the payload actually handed over?
		used_payload=$(jq -sr --arg final "$final" '
			[.[] | select(.signpost_shown) | .payload_paths // [] | .[]] as $offered |
			(try ($final | capture("LOCATION[[:space:]]+(?<path>[^[:space:]:]+):(?<line>[0-9]+)")) catch null) as $reported |
			if $reported == null then false else ($offered | any(. == $reported.path)) end' "$events") || return
	fi
	correct=$(jq -nr --arg final "$final" --argjson acceptance "$acceptance" '
		(try ($final | capture("LOCATION[[:space:]]+(?<path>[^[:space:]:]+):(?<line>[0-9]+)")) catch null) as $reported |
		if $reported == null then false else $acceptance.accepted | any(.path == $reported.path and (.line_min <= ($reported.line|tonumber)) and (.line_max >= ($reported.line|tonumber))) end') || return

	adoption_kind=ran-stack-command
	adopted=$ran_stack
	if [[ $condition == payload-signpost ]]; then
		adoption_kind=used-injected-location
		adopted=$used_payload
	fi
	signpost_correct=false
	if [[ $shown == true && $adopted == true && $correct == true ]]; then
		signpost_correct=true
	fi
	jq -nc --argjson assignment "$assignment" --argjson shown "$shown" --argjson adopted "$adopted" \
		--argjson ran_stack "$ran_stack" --argjson used_payload "$used_payload" --arg adoption_kind "$adoption_kind" \
		--argjson families "$families" \
		--argjson correct "$correct" --argjson spcorrect "$signpost_correct" --argjson turns "$turns" --argjson tokens "$tokens" \
		'{assignment_id:$assignment.assignment_id,task_id:$assignment.task_id,class:$assignment.class,model_family:$assignment.model_family,condition:$assignment.condition,signpost_shown:$shown,adopted:$adopted,adoption_kind:$adoption_kind,ran_stack_command:$ran_stack,used_injected_location:$used_payload,intent_families:$families,correct:$correct,signpost_correct:$spcorrect,turns_to_locate:$turns,tokens_to_resolution:$tokens}' || return
}

completed=0
while IFS= read -r assignment; do
	((limit > 0 && completed >= limit)) && break
	id=$(jq -r .assignment_id <<<"$assignment")
	if jq -e --arg id "$id" 'select(.assignment_id==$id)' "$results" >/dev/null 2>&1; then
		continue
	fi
	model=$(jq -r .model_family <<<"$assignment")
	condition=$(jq -r .condition <<<"$assignment")
	query=$(jq -r .query <<<"$assignment")
	task_id=$(jq -r .task_id <<<"$assignment")
	acceptance=$(jq -cs --arg task "$task_id" '[.[] | select(.task_id==$task)] | if length == 1 then .[0] else error("acceptance set missing or duplicated for " + $task) end' "$acceptance_sets") || exit 3
	repo_slug=$(jq -r .repo <<<"$assignment")
	repo=${repo_slug##*/}
	revision=$(jq -r .revision <<<"$assignment")
	repo_dir=$corpus_root/$repo
	if [[ $(git -C "$repo_dir" rev-parse HEAD 2>/dev/null) != "$revision" ]]; then
		printf '%s: corpus revision mismatch for %s\n' "$id" "$repo" >&2
		exit 3
	fi
	raw=$artifacts/raw/$id.jsonl
	events=$artifacts/events/$id.jsonl
	settings=$artifacts/settings/$id.json
	: > "$events"
	prompt=$(prompt_for "$query" "$condition")
	hook_cmd=$(printf 'env DP_SIGNPOST_CONDITION=%q DP_SIGNPOST_TASK_ID=%q DP_SIGNPOST_MODEL_FAMILY=%q DP_SIGNPOST_REPO=%q DP_SIGNPOST_LOG=%q DP_SIGNPOST_BOBBIN_URL=%q DP_SIGNPOST_TIMEOUT_MS=%q %q signpost' "$condition" "$(jq -r .task_id <<<"$assignment")" "$model" "$repo" "$events" "$bobbin_server/search" "$signpost_timeout_ms" "$dp_bin")
	prefetch_cmd=$(printf 'env DP_SIGNPOST_REPO=%q DP_SIGNPOST_BOBBIN_URL=%q %q signpost-prefetch' "$repo" "$bobbin_server/search" "$dp_bin")

	# BOBBIN_SERVER points the AGENT's own bobbin at the evaluation index. Without
	# it the agent's `bobbin search` goes wherever this host's bobbin is
	# configured to go, which for two of the five pinned corpora is a backend
	# that does not index them: adoption would then be measured against an index
	# that cannot answer, and the arm would look useless for a reason that has
	# nothing to do with signposting.
	env_args=(env "DP_EVAL_BOBBIN_BIN=$bobbin_bin" "DP_EVAL_BOBBIN_SERVER=$bobbin_server" \
		"DP_EVAL_REPO=$repo" "BOBBIN_SERVER=$bobbin_server")
	if [[ $condition == replacement ]]; then
		env_args+=("PATH=$artifacts/replacement-bin:$PATH")
	fi
	status=0
	if [[ $model == codex ]]; then
		args=(codex exec --ephemeral --ignore-user-config --ignore-rules --sandbox read-only --json -C "$repo_dir")
		if [[ $condition == always-signpost || $condition == gated-signpost || $condition == payload-signpost ]]; then
			escaped=${hook_cmd//\/\\}; escaped=${escaped//\"/\\\"}
			pre_escaped=${prefetch_cmd//\/\\}; pre_escaped=${pre_escaped//\"/\\\"}
			args+=(--dangerously-bypass-hook-trust -c "hooks.PreToolUse=[{matcher=\"Bash\",hooks=[{type=\"command\",command=\"$pre_escaped\"}]}]" -c "hooks.PostToolUse=[{matcher=\"Bash\",hooks=[{type=\"command\",command=\"$escaped\"}]}]")
		fi
		"${env_args[@]}" "${args[@]}" "$prompt" < /dev/null > "$raw" || status=$?
	else
		# PERMISSIONS ARE PART OF THE INSTRUMENT, NOT OF THE ENVIRONMENT.
		#
		# `--permission-mode dontAsk` with `--setting-sources ''` loads no
		# permission rules at all, so Claude DENIES Bash calls it does not
		# auto-approve — and an agent that cannot run its search produces a row
		# that scores as a failure of the intervention. Measured on the first
		# main matrix: 38 of 189 completed runs and ALL 11 that failed to score
		# carried "denied because Claude Code is running in don't ask mode", and
		# every one of the 49 was claude, none codex. That is not a model
		# difference, it is one arm of the experiment being unable to act.
		#
		# So the settings file is written for EVERY claude condition, carrying
		# the Bash allow, and the hooks are added only where the arm calls for
		# them. Previously the baselines got no settings file at all, which is
		# why the contamination reached conditions that install no hooks.
		hooks='{}'
		if [[ $condition == always-signpost || $condition == gated-signpost || $condition == payload-signpost ]]; then
			hooks=$(jq -n --arg pre "$prefetch_cmd" --arg post "$hook_cmd" '{PreToolUse:[{matcher:"Bash",hooks:[{type:"command",command:$pre,timeout:5}]}],PostToolUse:[{matcher:"Bash",hooks:[{type:"command",command:$post,timeout:5}]}]}')
		fi
		jq -n --argjson hooks "$hooks" '{permissions:{allow:["Bash"]}, hooks:$hooks}' > "$settings"
		args=(claude -p --no-session-persistence --permission-mode dontAsk --tools Bash --disable-slash-commands --setting-sources '' --output-format stream-json --verbose --settings "$settings")
		(cd "$repo_dir" && "${env_args[@]}" "${args[@]}" "$prompt" < /dev/null) > "$raw" || status=$?
	fi
	if ((status != 0)); then
		printf '%s: %s/%s failed with exit %d\n' "$id" "$model" "$condition" "$status" >&2
		mv "$raw" "$raw.failed"
		continue
	fi
	if ! score_one "$assignment" "$acceptance" "$raw" "$events" "$model" "$condition" >> "$results"; then
		printf '%s: scoring failed for %s/%s\n' "$id" "$model" "$condition" >&2
		mv "$raw" "$raw.failed"
		continue
	fi
	completed=$((completed + 1))
	printf '%s: completed %s/%s (%d new)\n' "$id" "$model" "$condition" "$completed" >&2
done < "$assignments"

printf 'results: %s (%s rows)\n' "$results" "$(wc -l < "$results")" >&2
