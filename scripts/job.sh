#!/usr/bin/env bash
# Usage: scripts/job.sh [-C DIR] [-i IMAGE] [-t TIMEOUT_SECONDS] -- COMMAND...
# Syncs DIR (creating the workspace if needed), runs COMMAND remotely, streams the log,
# and exits with the job's exit code (124 on timeout, 125 on infrastructure error).
#   scripts/job.sh -- make test
#   scripts/job.sh -i busybox:1.37 -- sh -c 'ls -la'
source "$(dirname "$0")/lib.sh"
scripts="$(cd "$(dirname "$0")" && pwd)"

dir=. image="${RWE_IMAGE:-golang:1.27}" timeout=0
while getopts "C:i:t:" opt; do
	case "$opt" in
	C) dir="$OPTARG" ;;
	i) image="$OPTARG" ;;
	t) timeout="$OPTARG" ;;
	*) die "usage: $0 [-C DIR] [-i IMAGE] [-t TIMEOUT_SECONDS] -- COMMAND..." ;;
	esac
done
shift $((OPTIND - 1))
[[ $# -gt 0 ]] || die "no command given (usage: $0 [-C DIR] [-i IMAGE] -- COMMAND...)"
dir="$(cd "$dir" && pwd)"

[[ -s "$(state_file "$dir")" ]] || "$scripts/create.sh" "$dir" >/dev/null
synced=$("$scripts/sync.sh" "$dir")
ws=$(cat "$(state_file "$dir")")
hash=$(awk '/expected_hash/ {print $2}' <<<"$synced")

body=$(jq -nc --arg h "$hash" --arg i "$image" --argjson t "$timeout" --argjson c "$(json_array "$@")" \
	'{expected_hash: $h, image: $i, command: $c, timeout_seconds: $t}')
job=$(api POST "/workspaces/$ws/jobs" -H 'Content-Type: application/json' --data-binary "$body")
id=$(jq -r .id <<<"$job")
echo "==> job $id: $image $*" >&2

final=""
while IFS= read -r line; do
	case "$line" in
	"event: done") read -r line && final="${line#data: }" ;;
	"data: "*) printf '%s\n' "${line#data: }" ;;
	esac
done < <(curl -sSN "$RWE_URL/jobs/$id/logs")

[[ -n "$final" ]] || final=$(api GET "/jobs/$id")
status=$(jq -r .status <<<"$final")
echo "==> $status in $(jq -r '.duration_ms // 0' <<<"$final")ms $(jq -r '.error // empty' <<<"$final")" >&2

case "$status" in
passed) exit 0 ;;
failed) exit "$(jq -r .exit_code <<<"$final")" ;;
timeout) exit 124 ;;
*) exit 125 ;;
esac
