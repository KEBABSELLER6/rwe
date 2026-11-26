#!/usr/bin/env bash
# Usage: scripts/sync.sh [DIR] [WORKSPACE_ID]
# Syncs DIR to the workspace (default: id from DIR/.rwe/workspace) and uploads missing blobs.
source "$(dirname "$0")/lib.sh"

dir="$(cd "${1:-.}" && pwd)"
id="${2:-$(cat "$(state_file "$dir")" 2>/dev/null)}" || true
[[ -n "$id" ]] || die "no workspace id; run scripts/create.sh first"

manifest=$(manifest_json "$dir")
missing=$(jq -c '{files: .}' <<<"$manifest" |
	api POST "/workspaces/$id/sync" -H 'Content-Type: application/json' --data-binary @- |
	jq -r '.missing[]')

declare -A sent=()
uploaded=0
remaining=0
while IFS= read -r path; do
	[[ -n "$path" ]] || continue
	sum=$(jq -r --arg p "$path" '.[$p]' <<<"$manifest")
	[[ -z "${sent[$sum]:-}" ]] || continue
	sent[$sum]=1
	uploaded=$((uploaded + 1))
	remaining=$(api PUT "/workspaces/$id/blobs/$sum" \
		-H 'Content-Type: application/octet-stream' --data-binary @"$dir/$path" | jq -r .remaining)
	echo "  uploaded $path (remaining: $remaining)"
done <<<"$missing"

echo "workspace:     $id"
echo "files:         $(jq length <<<"$manifest")"
echo "uploaded:      $uploaded blob(s)"
echo "expected_hash: $(manifest_hash <<<"$manifest")"
[[ "$remaining" == 0 ]] || die "server still waiting for $remaining file(s)"
