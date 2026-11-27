#!/usr/bin/env bash
# Usage: scripts/create.sh [DIR]   (default: current dir)
# Uploads DIR as a new workspace and remembers its id in DIR/.rwe/workspace.
source "$(dirname "$0")/lib.sh"

dir="$(cd "${1:-.}" && pwd)"

res=$(list_files "$dir" | tar --null -C "$dir" -czf - -T - |
	api POST /workspaces -H 'Content-Type: application/gzip' --data-binary @-)

id=$(jq -r .id <<<"$res")
mkdir -p "$dir/.rwe"
echo "$id" >"$(state_file "$dir")"

echo "workspace:     $id"
echo "manifest_hash: $(jq -r .manifest_hash <<<"$res")"
