#!/usr/bin/env bash
# Usage: scripts/smoke.sh   (needs a running server, e.g. `make run`)
# Runs create -> edit -> sync against a throwaway directory and checks the results.
source "$(dirname "$0")/lib.sh"
scripts="$(cd "$(dirname "$0")" && pwd)"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

pass() { echo "ok   $*"; }
fail() { echo "FAIL $*" >&2; exit 1; }

expect_status() {
	local want="$1" desc="$2"; shift 2
	local got
	got=$(curl -sS -o /dev/null -w '%{http_code}' "$@")
	[[ "$got" == "$want" ]] && pass "$desc -> $got" || fail "$desc -> $got, want $want"
}

mkdir -p "$work/src/pkg" "$work/old"
echo 'module smoke' >"$work/go.mod"
echo 'package pkg' >"$work/src/pkg/a.go"
echo 'same' >"$work/dup1.txt"
echo 'gone' >"$work/old/remove-me.txt"
printf '#!/bin/sh\necho hi\n' >"$work/run.sh" && chmod +x "$work/run.sh"

out=$("$scripts/create.sh" "$work")
id=$(cat "$(state_file "$work")")
remote_hash=$(awk '/manifest_hash/ {print $2}' <<<"$out")
local_hash=$(manifest_json "$work" | manifest_hash)
[[ "$remote_hash" == "$local_hash" ]] && pass "create: server hash matches local" || fail "create: hash $remote_hash != $local_hash"

out=$("$scripts/sync.sh" "$work")
grep -q 'uploaded:      0' <<<"$out" && pass "sync without changes uploads nothing" || fail "no-op sync uploaded: $out"

echo 'package pkg // changed' >"$work/src/pkg/a.go"
echo 'same' >"$work/dup2.txt"
mkdir -p "$work/new" && echo 'brand new' >"$work/new/file.txt"
rm -r "$work/old"
out=$("$scripts/sync.sh" "$work")
grep -q "uploaded:      3" <<<"$out" && pass "sync after edits uploads only changed paths (3 blobs)" || fail "sync: $out"

out=$("$scripts/sync.sh" "$work")
grep -q 'uploaded:      0' <<<"$out" && pass "second sync is a no-op" || fail "second sync: $out"

expect_status 400 "create with wrong content-type" -X POST -H 'Content-Type: text/plain' --data 'x' "$RWE_URL/workspaces"
expect_status 400 "create with non-gzip body" -X POST -H 'Content-Type: application/gzip' --data 'x' "$RWE_URL/workspaces"
expect_status 400 "sync with bad id" -X POST -H 'Content-Type: application/json' --data '{"files":{}}' "$RWE_URL/workspaces/nope/sync"
expect_status 404 "sync unknown workspace" -X POST -H 'Content-Type: application/json' --data '{"files":{}}' "$RWE_URL/workspaces/00000000-0000-7000-8000-000000000000/sync"
expect_status 400 "sync with typo'd key" -X POST -H 'Content-Type: application/json' --data '{"file":{}}' "$RWE_URL/workspaces/$id/sync"
expect_status 400 "sync with escaping path" -X POST -H 'Content-Type: application/json' --data "{\"files\":{\"../x\":\"$(printf x | sha256sum | cut -c1-64)\"}}" "$RWE_URL/workspaces/$id/sync"

sum=$(printf 'never requested' | sha256sum | cut -c1-64)
expect_status 400 "blob nobody asked for" -X PUT -H 'Content-Type: application/octet-stream' --data 'never requested' "$RWE_URL/workspaces/$id/blobs/$sum"

out=$("$scripts/sync.sh" "$work")
grep -q 'uploaded:      0' <<<"$out" && pass "workspace still intact after rejected requests" || fail "workspace changed: $out"


# --- jobs (needs docker on the server; uses a small image)
img=busybox:1.37
hash=$("$scripts/sync.sh" "$work" | awk '/expected_hash/ {print $2}')
job_body() { jq -nc --arg h "$1" --arg i "$img" --argjson c "$(json_array "${@:2}")" '{expected_hash: $h, image: $i, command: $c}'; }

set +e
out=$("$scripts/job.sh" -C "$work" -i "$img" -- sh -c 'cat go.mod; exit 3' 2>/dev/null)
code=$?
set -e
[[ "$code" == 3 ]] && pass "job exit code passed through (3)" || fail "job exit code $code, want 3"
grep -q 'module smoke' <<<"$out" && pass "job sees workspace files and streams logs" || fail "job output: $out"

out=$("$scripts/job.sh" -C "$work" -i "$img" -- sh -c 'echo built > out.txt; ls' 2>/dev/null)
grep -q 'out.txt' <<<"$out" && pass "job can write into the workspace" || fail "job output: $out"

expect_status 409 "job with stale expected_hash" -X POST -H 'Content-Type: application/json' --data "$(job_body "$(printf '0%.0s' {1..64})" true)" "$RWE_URL/workspaces/$id/jobs"
expect_status 400 "job with typo'd key" -X POST -H 'Content-Type: application/json' --data "{\"expected_hash\":\"$hash\",\"image\":\"$img\",\"command\":[\"true\"],\"timout_seconds\":5}" "$RWE_URL/workspaces/$id/jobs"
expect_status 400 "job without command" -X POST -H 'Content-Type: application/json' --data "$(job_body "$hash")" "$RWE_URL/workspaces/$id/jobs"
expect_status 404 "unknown job" "$RWE_URL/jobs/00000000-0000-7000-8000-000000000000"

hash=$("$scripts/sync.sh" "$work" | awk '/expected_hash/ {print $2}')
running=$(api POST "/workspaces/$id/jobs" -H 'Content-Type: application/json' --data-binary "$(job_body "$hash" sleep 3)" | jq -r .id)
expect_status 409 "sync while job runs" -X POST -H 'Content-Type: application/json' --data '{"files":{}}' "$RWE_URL/workspaces/$id/sync"
curl -sN "$RWE_URL/jobs/$running/logs" >/dev/null
[[ $(api GET "/jobs/$running" | jq -r .status) == passed ]] && pass "busy job finished" || fail "busy job did not pass"
"$scripts/sync.sh" "$work" >/dev/null && pass "workspace released after job" || fail "sync after job failed"

echo "all smoke checks passed (workspace $id)"
