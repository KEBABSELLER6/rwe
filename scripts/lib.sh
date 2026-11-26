#!/usr/bin/env bash
set -euo pipefail

RWE_URL="${RWE_URL:-http://localhost:8999}"

die() { echo "error: $*" >&2; exit 1; }

# list_files DIR: NUL-separated regular files, relative to DIR.
# Git repos: tracked + untracked-but-not-ignored. Otherwise: every file.
# Always skips .rwe/ (local state) and .git/.
list_files() {
	local dir="$1"
	(
		cd "$dir"
		if git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
			git ls-files -z --cached --others --exclude-standard
		else
			find . -type f -printf '%P\0'
		fi
	) | while IFS= read -r -d '' f; do
		case "$f" in .rwe/* | .git/*) continue ;; esac
		[[ -f "$dir/$f" && ! -L "$dir/$f" ]] && printf '%s\0' "$f"
	done
}

# manifest_json DIR: {"path": "sha256", ...}
manifest_json() {
	local dir="$1"
	(cd "$dir" && list_files . | xargs -0 --no-run-if-empty sha256sum --) |
		jq -Rn '[inputs | capture("^(?<h>[0-9a-f]{64})  (?<p>.*)$") | {(.p): .h}] | add // {}'
}

# manifest_hash: same algorithm as manifest.Manifest.Hash, reads manifest JSON on stdin.
manifest_hash() {
	jq -j 'to_entries | sort_by(.key) | .[] | "\(.key)\u0000\(.value)\n"' | sha256sum | cut -c1-64
}

# api METHOD PATH [curl args...]: prints body, fails with the server message on non-2xx.
api() {
	local method="$1" path="$2"; shift 2
	local out code
	out=$(curl -sS -X "$method" -w '\n%{http_code}' "$@" "$RWE_URL$path") || die "cannot reach $RWE_URL (is the server running? make run)"
	code="${out##*$'\n'}"
	out="${out%$'\n'*}"
	if [[ "$code" != 2* ]]; then
		die "$method $path -> $code $out"
	fi
	printf '%s\n' "$out"
}

state_file() { echo "$1/.rwe/workspace"; }
