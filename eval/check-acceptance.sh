#!/usr/bin/env bash
# Adjudication gate for the task set. Run it BEFORE any assignment burns quota.
#
# WHY THIS EXISTS. The 2026-08-27 pilot's correctness numbers were not
# publication-safe because the acceptance fixtures omitted valid nearby and
# caller locations: a model that answered correctly was scored wrong, and
# nothing in the harness could tell. An acceptance set is data like any other,
# and data nobody checked against the corpus is a guess with a schema.
#
# It asserts, per task, at the PINNED revision:
#   1. the corpus is at the revision the task names;
#   2. every ground_truth path exists and the line is inside the file;
#   3. every ground_truth line falls inside at least one accepted range —
#      an acceptance set that excludes its own ground truth scores the right
#      answer wrong, which is the exact defect being guarded;
#   4. every task has exactly one acceptance set, marked adjudicated.
# It prints the source line at each ground truth so the mapping can be read by
# a person rather than trusted.
#
# usage: eval/check-acceptance.sh --corpus-root DIR [--tasks F] [--acceptance F] [--quiet]
set -uo pipefail

corpus_root=; tasks=eval/tasks.jsonl; acceptance=eval/acceptance-sets.jsonl; quiet=0
while (($#)); do
	case $1 in
	--corpus-root) corpus_root=$2; shift 2 ;;
	--tasks) tasks=$2; shift 2 ;;
	--acceptance) acceptance=$2; shift 2 ;;
	--quiet) quiet=1; shift ;;
	-h | --help) sed -n '2,20p' "$0"; exit 0 ;;
	*) printf 'unknown argument: %s\n' "$1" >&2; exit 2 ;;
	esac
done
[[ -n $corpus_root ]] || { printf 'missing --corpus-root\n' >&2; exit 2; }
for f in "$tasks" "$acceptance"; do
	[[ -r $f ]] || { printf 'unreadable: %s\n' "$f" >&2; exit 2; }
done
command -v jq >/dev/null || { printf 'jq is required\n' >&2; exit 2; }

fail=0
note() { printf '%s\n' "$*" >&2; fail=1; }

while IFS= read -r task; do
	id=$(jq -r .task_id <<<"$task")
	slug=$(jq -r .repo <<<"$task")
	revision=$(jq -r .revision <<<"$task")
	repo=${slug##*/}
	dir=$corpus_root/$repo

	head=$(git -C "$dir" rev-parse HEAD 2>/dev/null)
	[[ $head == "$revision" ]] || { note "$id: $repo is at ${head:-<missing>}, task pins $revision"; continue; }

	sets=$(jq -c --arg id "$id" 'select(.task_id==$id)' "$acceptance")
	count=$(wc -l <<<"$sets"); [[ -z $sets ]] && count=0
	if ((count != 1)); then
		note "$id: $count acceptance sets, want exactly 1"
		continue
	fi
	[[ $(jq -r .adjudicated_before_campaign <<<"$sets") == true ]] ||
		note "$id: acceptance set is not marked adjudicated_before_campaign"

	while IFS= read -r truth; do
		path=${truth%:*}; line=${truth##*:}
		file=$dir/$path
		if [[ ! -r $file ]]; then note "$id: no such file at the pinned revision: $path"; continue; fi
		total=$(wc -l <"$file")
		if ((line < 1 || line > total)); then note "$id: $path:$line is outside the file (1..$total)"; continue; fi
		inside=$(jq -r --arg p "$path" --argjson l "$line" \
			'[.accepted[] | select(.path == $p and .line_min <= $l and .line_max >= $l)] | length' <<<"$sets")
		((inside > 0)) || note "$id: ground truth $path:$line is NOT inside any accepted range"
		((quiet)) || printf '%-26s %-46s %s\n' "$id" "$path:$line" "$(sed -n "${line}p" "$file" | cut -c1-70)"
	done < <(jq -r '.ground_truth[]' <<<"$task")
done < <(jq -c '.' "$tasks")

tasks_n=$(wc -l <"$tasks")
sets_n=$(wc -l <"$acceptance")
((tasks_n == sets_n)) || note "$tasks_n tasks but $sets_n acceptance sets"
if ((fail)); then
	printf 'ADJUDICATION FAILED — do not spend the envelope\n' >&2
	exit 1
fi
printf 'adjudication ok: %d tasks, %d acceptance sets, every ground truth inside its range\n' "$tasks_n" "$sets_n" >&2
