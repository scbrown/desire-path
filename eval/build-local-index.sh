#!/usr/bin/env bash
# Build the LOCAL RESIDENT semantic index the signpost hook needs.
#
# WHY THIS EXISTS. Signpost delivery is gated by a 150 ms PostToolUse contract
# (5 s when the PreToolUse prefetch runs concurrently). A remote/shared search
# backend measured p50 5.95 s on this workload — 40x the direct contract and
# past even the prefetch budget — so a resident index is not an optimisation of
# signposting, it is the only architecture in which signposts can be delivered
# at all. Certification runs against THIS, not against a shared backend.
#
# ONNX RUNTIME. `bobbin index` embeds locally through ort 2.0.0-rc.11, which
# dlopen()s `libonnxruntime.so` by BARE NAME and panics if the dynamic loader
# cannot find it:
#
#   thread 'main' panicked at ort-2.0.0-rc.11/src/lib.rs:191:41:
#   Failed to load ONNX Runtime dylib: ... "failed to load from
#   `libonnxruntime.so`: dlopen failed"
#
# The fix is ORT_DYLIB_PATH pointing at a REAL FILE (not a directory, and not a
# bare soname): ort passes that value straight to dlopen. Any onnxruntime >= the
# version ort was built against works; measured here with 1.29.0.
#
#   ORT_DYLIB_PATH=$HOME/.local/lib/onnxruntime/libonnxruntime.so
#
# usage:
#   eval/build-local-index.sh --corpus-root DIR --index DIR [--corpora FILE]
set -euo pipefail

usage() { sed -n '2,30p' "$0"; exit "${1:-2}"; }

corpus_root=; index=; corpora=eval/corpora.json
while (($#)); do
	case $1 in
	--corpus-root) corpus_root=$2; shift 2 ;;
	--index) index=$2; shift 2 ;;
	--corpora) corpora=$2; shift 2 ;;
	-h | --help) usage 0 ;;
	*) usage >&2 ;;
	esac
done
for required in corpus_root index; do
	[[ -n ${!required} ]] || { printf 'missing --%s\n' "${required//_/-}" >&2; exit 2; }
done
[[ -r $corpora ]] || { printf 'no corpora file: %s\n' "$corpora" >&2; exit 2; }
command -v jq >/dev/null || { printf 'jq is required\n' >&2; exit 2; }
command -v bobbin >/dev/null || { printf 'bobbin is required\n' >&2; exit 2; }

# Fail closed on the loader rather than indexing zero chunks and calling it a
# corpus: a missing dylib is the failure this whole script exists to prevent.
: "${ORT_DYLIB_PATH:=$HOME/.local/lib/onnxruntime/libonnxruntime.so}"
export ORT_DYLIB_PATH
[[ -r $ORT_DYLIB_PATH && ! -d $ORT_DYLIB_PATH ]] || {
	printf 'ORT_DYLIB_PATH must name a readable libonnxruntime file: %s\n' "$ORT_DYLIB_PATH" >&2
	exit 3
}
export RUST_LOG=${RUST_LOG:-warn}

mkdir -p "$index"
[[ -d $index/.bobbin ]] || (cd "$index" && bobbin init >/dev/null)

# ONE index holding every corpus, with an explicit --repo name per corpus. The
# hook scopes its query with ?repo=<name>, so the names here MUST equal the
# `repo` field of the replay workload rows; a mismatch returns zero candidates,
# which is indistinguishable from a genuine retrieval miss.
while IFS= read -r entry; do
	slug=$(jq -r .repo <<<"$entry")
	revision=$(jq -r .revision <<<"$entry")
	name=${slug##*/}
	src=$corpus_root/$name
	[[ -d $src ]] || { printf 'missing corpus: %s\n' "$src" >&2; exit 3; }
	head=$(git -C "$src" rev-parse HEAD 2>/dev/null || true)
	[[ $head == "$revision" ]] || {
		printf 'corpus revision mismatch for %s: want %s got %s\n' "$name" "$revision" "${head:-<not a git repo>}" >&2
		exit 3
	}
	started=$(date +%s)
	(cd "$index" && bobbin index --repo "$name" --source "$src" --quiet >/dev/null)
	printf '%s: indexed at %s in %ds\n' "$name" "${revision:0:12}" "$(($(date +%s) - started))" >&2
done < <(jq -c '.[]' "$corpora")

printf 'index: %s\nserve: ORT_DYLIB_PATH=%s bobbin serve --http --port 3030 %s\n' \
	"$index" "$ORT_DYLIB_PATH" "$index" >&2
