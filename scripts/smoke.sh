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

echo "all smoke checks passed (workspace $id)"
