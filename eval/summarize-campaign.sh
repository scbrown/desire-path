#!/usr/bin/env bash
# Summarise a campaign's results into the tables the preregistration fixed.
#
# It is a script and not a set of ad-hoc jq lines because the preregistration
# names the endpoints in advance, and a summary retyped by hand after seeing the
# numbers is how post-hoc analysis gets in without anyone deciding to do it.
#
# IT REFUSES TO POOL ADOPTION ACROSS KINDS. A pointer arm is adopted by RUNNING
# the stack command; the payload arm has already run it, so it is adopted by
# USING a location it was handed. Those are different acts and their rates are
# not summable. Cells are reported per (model, condition), and any cell whose
# rows disagree about `adoption_kind` is an error, not an average.
#
# usage: eval/summarize-campaign.sh --results FILE [--results FILE ...]
set -uo pipefail

results=()
while (($#)); do
	case $1 in
	--results) results+=("$2"); shift 2 ;;
	-h | --help) sed -n '2,14p' "$0"; exit 0 ;;
	*) printf 'unknown argument: %s\n' "$1" >&2; exit 2 ;;
	esac
done
((${#results[@]})) || { printf 'missing --results\n' >&2; exit 2; }
for f in "${results[@]}"; do [[ -r $f ]] || { printf 'unreadable: %s\n' "$f" >&2; exit 2; }; done
command -v jq >/dev/null || { printf 'jq is required\n' >&2; exit 2; }

cat "${results[@]}" | jq -s '
	def rate($n; $d): if $d == 0 then null else (($n / $d) * 1000 | round) / 1000 end;
	def mean(f): if length == 0 then null else ((map(f) | add) / length * 100 | round) / 100 end;

	. as $all |
	{
	  runs: ($all | length),

	  # THE PREREGISTERED PRIMARY DENOMINATOR: assignments that actually saw a
	  # signpost. An adoption rate over all runs would be diluted by arms that
	  # never emit, which is how a pilot reported "adoption is zero" from a
	  # configuration in which almost nothing was ever shown.
	  model_visible_signposts: ($all | map(select(.signpost_shown)) | length),

	  cells: ($all | group_by(.model_family + " " + .condition) | map(
	    (map(.adoption_kind // "") | unique - [""]) as $kinds |
	    if ($kinds | length) > 1 then
	      error("cell " + .[0].model_family + "/" + .[0].condition + " mixes adoption kinds: " + ($kinds | join(", ")))
	    else . end |
	    (map(select(.signpost_shown)) | length) as $shown |
	    {
	      model_family: .[0].model_family,
	      condition: .[0].condition,
	      adoption_kind: ($kinds[0] // null),
	      runs: length,
	      shown: $shown,
	      adoption_rate: rate((map(select(.adopted)) | length); $shown),
	      ran_stack_command: (map(select(.ran_stack_command)) | length),
	      used_injected_location: (map(select(.used_injected_location)) | length),
	      correctness_rate: rate((map(select(.correct)) | length); length),
	      signpost_precision: rate((map(select(.signpost_correct)) | length); $shown),
	      avg_turns: mean(.turns_to_locate),
	      avg_tokens: mean(.tokens_to_resolution)
	    })),

	  by_class: ($all | group_by(.class) | map({
	      class: .[0].class, runs: length,
	      correctness_rate: rate((map(select(.correct)) | length); length)})),

	  # Reported, NOT powered: the campaign blocks over tasks, not over families.
	  intent_families: ($all | map(.intent_families // [] | .[]) | group_by(.) |
	      map({family: .[0], signposts: length}))
	}'
