#!/usr/bin/env bash
# Generate the intent-family replay workload by RUNNING each command in its
# pinned corpus and recording the real line count.
#
# WHY IT RUNS THEM. `result_cardinality` drives the gating predicate, so a made-up
# number produces a made-up predicate and the replay measures a workload that
# never existed. The commands below are shaped after `find -name` and
# `git log -S|--grep` invocations agents actually run — mined from the desire
# corpus, where find appears in dozens of failed calls and git log in 154 — and
# rewritten against the pinned corpora, because the real ones carry host paths
# that must not enter a public repository.
#
# ⚠ The history rows need corpora with REAL COMMIT HISTORY. A `--depth 1` clone
# has one commit, so `git log -S` finds nothing and the index holds no commit
# chunks: the family would measure zero for a reason that is an artifact of the
# clone. This script REFUSES to emit history rows from a shallow corpus rather
# than emit rows that would certify a family against an empty history.
#
# usage: eval/build-family-workload.sh --corpus-root DIR > eval/replay-workload-families.jsonl
set -uo pipefail

corpus_root=
while (($#)); do
	case $1 in
	--corpus-root) corpus_root=$2; shift 2 ;;
	-h | --help) sed -n '2,20p' "$0"; exit 0 ;;
	*) printf 'unknown argument: %s\n' "$1" >&2; exit 2 ;;
	esac
done
[[ -n $corpus_root ]] || { printf 'missing --corpus-root\n' >&2; exit 2; }
command -v jq >/dev/null || { printf 'jq is required\n' >&2; exit 2; }

# repo <TAB> command. The command is run with the corpus as cwd.
rows=$(cat <<'ROWS'
cobra	find . -name '*_completions.go'
cobra	find . -name 'flag_groups*.go'
cobra	find . -path '*/cobra*' -name 'args.go'
desire-path	find . -name 'pave_*.go'
desire-path	find . -name '*signpost*'
desire-path	find . -name 'claudecode*.go'
beads	find . -path '*storage/dolt*' -name 'circuit*.go'
beads	find . -name 'doltserver*.go'
beads	find . -name 'githooksenv*.go'
gastown	find . -name 'escalate*.go'
gastown	find . -name 'polecat_spawn.go'
gastown	find . -name 'crew_lifecycle.go'
ripgrep	find . -name 'hiargs.rs'
ripgrep	find . -path '*printer*' -name 'json.rs'
ripgrep	find . -name 'walk.rs'
cobra	git log --oneline -SExactArgs
cobra	git log --oneline --grep=completion
desire-path	git log --oneline -Ssignpost
desire-path	git log --oneline --grep=pave
beads	git log --oneline -SErrCircuitOpen
beads	git log --oneline --grep='circuit breaker'
gastown	git log --oneline -SSpawnPolecatForSling
gastown	git log --oneline --grep=escalation
ripgrep	git log --oneline -SBinaryDetection
ripgrep	git log --oneline --grep=pcre2
ROWS
)

shallow_warned=""
while IFS=$'\t' read -r repo command; do
	[[ -n $repo ]] || continue
	dir=$corpus_root/$repo
	[[ -d $dir ]] || { printf 'missing corpus: %s\n' "$dir" >&2; exit 3; }
	if [[ $command == git\ log* ]]; then
		if [[ $(git -C "$dir" rev-parse --is-shallow-repository 2>/dev/null) == true ]]; then
			[[ $shallow_warned == *"$repo"* ]] || {
				printf 'skipping history rows for %s: shallow clone, run `git -C %s fetch --unshallow` first\n' "$repo" "$dir" >&2
				shallow_warned="$shallow_warned $repo"
			}
			continue
		fi
	fi
	count=$( (cd "$dir" && eval "$command" 2>/dev/null | wc -l) )
	tool=find
	[[ $command == git\ log* ]] && tool=git-log
	# The query the hook will extract is not restated here: the point of the
	# replay is that the PARSER derives it from the command, so writing it in by
	# hand would test the workload instead of the parser.
	jq -cn --arg command "$command" --arg repo "$repo" --arg tool "$tool" --argjson n "$count" \
		'{command:$command, repo:$repo, tool:$tool, result_cardinality:$n,
		  predicate: (if $n == 0 then "null" elif $n > 100 then "high-cardinality" else "none" end)}'
done <<<"$rows"
