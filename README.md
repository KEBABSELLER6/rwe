# rwe — Remote Workflow Engine

Run your builds and tests on a spare machine instead of your laptop.

A coding agent edits files locally, then calls rwe to verify the change: rwe syncs the working tree
to a server on your network, runs a command (e.g. `make test`) in a fresh Docker container there,
and returns the exit code and log. Your laptop stays free; the old box in the corner does the work.

```
 laptop                                   box on your network
┌──────────────────────────┐   HTTP     ┌───────────────────────────────┐
│ agent ──MCP──► rwe mcp   │ ─────────► │ server ──► docker run <image> │
│          or    rwe run   │  sync,     │    workspace files mounted    │
│                          │  job, logs │    at /workspace              │
└──────────────────────────┘ ◄───────── └───────────────────────────────┘
```

## How it works

- **Sync:** the first run uploads the project as a tarball. After that the client sends a manifest of
  `path → sha256` and uploads only the changed files. Files missing from the manifest are deleted
  on the server. Git projects send tracked plus untracked, non-ignored files.
- **Pinned jobs:** each job carries the manifest hash the client synced. The server rejects the job
  if the workspace doesn't match, so a job never runs on a mix of old and new files.
- **Jobs:** one at a time, queued on the server, each in a new container with the workspace mounted.
  Logs stream back live over SSE. If the connection drops, the client reconnects for up to 2 minutes
  and carries on where it left off.
- **Agents:** `rwe mcp` is a stdio MCP server with three tools: `run` (sync, run, return status and the
  last 100 log lines), `logs` (read any part of a job's log) and `stats` (recent runs, see below).
- **Run history:** every `rwe run` and MCP `run` adds a record to `.rwe/runs.jsonl` in the project:
  command, image, workspace hash, status, and how long sync, queue, copy, run, artifacts and cleanup
  took. The newest 50 are kept. `rwe stats [-n 20]` and the MCP `stats` tool summarize them. The
  history stays on your machine; nothing is sent anywhere.

## Quick start

```sh
make build                             # bin/server and bin/rwe
bin/server -addr :8999 -data ./data   # on the box, needs Docker
echo '{"server": "http://box:8999"}' > ~/.config/rwe/config.json   # on the laptop
bin/rwe run -- make test               # this repo verifying itself
```

Hooking it up to an agent, config files and precedence: see [`examples/`](examples/).

## Install

Each [release](https://github.com/KEBABSELLER6/rwe/releases) has the client for Linux and macOS
(`rwe-<os>-<arch>`), the server for Linux (`rwe-server-linux-<arch>`) and a `SHA256SUMS` file.
Download, check, install:

```sh
f=rwe-linux-amd64   # or rwe-linux-arm64, rwe-darwin-arm64, rwe-darwin-amd64
curl -fLO https://github.com/KEBABSELLER6/rwe/releases/latest/download/$f
curl -fLO https://github.com/KEBABSELLER6/rwe/releases/latest/download/SHA256SUMS
sha256sum -c --ignore-missing SHA256SUMS   # macOS: shasum -a 256 -c --ignore-missing SHA256SUMS
install -m 755 $f ~/.local/bin/rwe
rwe version
```

With the GitHub CLI, the first three lines are `gh release download -R KEBABSELLER6/rwe -p $f -p SHA256SUMS`
followed by the same checksum line. For the server, use `rwe-server-linux-<arch>` the same way.
With Go installed, `go install github.com/KEBABSELLER6/rwe/cmd/rwe@latest` builds the client into
`$(go env GOPATH)/bin` instead (`~/go/bin` by default). To build from a clone: `make build`.

`~/.local/bin` must be on your `PATH`; macOS doesn't add it by default. If `rwe version` still shows
the old version, either an older `rwe` comes first on `PATH` (`which -a rwe` lists them all) or your
shell remembers where it found `rwe` before (`hash -r`, or open a new shell). After upgrading,
restart your agent's MCP server so it picks up the new binary.

## What it is not

- **Not an agent sandbox or microVM platform.** rwe doesn't run agents. The agent stays on your
  machine; rwe only runs the commands it asks for.
- **Not a CI system.** No pipelines, triggers, caching or history. It's a fast "does this pass?" loop.
- **Not a security boundary.** Containers run as the server's user with `no-new-privileges` and a
  pids limit, and they _have network access_. Treat a job as code running on the box.
- **No authentication.** Anyone who can reach the port can run containers on the box. Keep it on a
  private network you control (LAN, Tailscale, …) and firewall it there. Auth is yours to add in front.
- **Docker only**, no Docker-in-Docker.

## Current state

Works today:

- Workspace create and diff sync, deletions, hash-pinned jobs
- Docker runner with timeout, memory and CPU limits, live log streaming
- `rwe run` CLI with the job's exit code (124 timeout, 125 infrastructure error)
- `rwe mcp` with `run`, `logs` and `stats` tools
- Run history in `.rwe/runs.jsonl`, shown by `rwe stats` and the MCP `stats` tool
- User and project config files, env overrides
- Cache directories kept between jobs (`caches` in `.rwe.json`); see [BENCHMARKS.md](BENCHMARKS.md)
- Artifacts: files a job leaves behind, downloaded to `.rwe/artifacts/<job>/` (`artifacts` in `.rwe.json`, `rwe run -a`, or the MCP `run` tool); see [examples/](examples/#artifacts)
- Cleanup on the box: old job logs, workspaces, shared caches and artifacts are deleted after a configurable time, and oversized artifacts are dropped; `0` turns each off. See [examples/](examples/#config-precedence)

Known limits:

- A server restart ends running jobs with status `error` and removes their containers. Queued jobs are queued again, unless their workspace changed or is gone.
- A sync with a running job gets 409: one job per workspace at a time.
- Only the exec bit of a file is synced, not its full mode (files are 0644 or 0755).
- Artifacts: only regular files and directories come back, no symlinks. Globs are one level deep (`*`, `?`, `[a-z]`), no `**`. Locally only the latest job's artifacts are kept.
- Caches have no size cap.

Planned: deployment notes.

## Development

```sh
make test    # go test -race ./...; e2e/ and Docker runner tests skip without Docker
make vet
make dist    # release binaries + SHA256SUMS in dist/
```
