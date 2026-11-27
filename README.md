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
  Logs stream back live over SSE.
- **Agents:** `rwe mcp` is a stdio MCP server with two tools: `run` (sync, run, return status and the
  last 100 log lines) and `logs` (read any part of a job's log).

## Quick start

```sh
make build                             # bin/server and bin/rwe
bin/server -addr :8999 -data ./data   # on the box, needs Docker
echo '{"server": "http://box:8999"}' > ~/.config/rwe/config.json   # on the laptop
bin/rwe run -- make test               # this repo verifying itself
```

Hooking it up to an agent, config files and precedence: see [`examples/`](examples/).

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
- `rwe mcp` with `run` and `logs` tools
- User and project config files, env overrides

Known limits:

- A server restart ends queued and running jobs with status `error`; finished jobs survive.
- Job logs are deleted 7 days after the job finishes (`RWE_LOG_RETENTION`); the job status is kept forever.
- A sync with a running job gets 409: one job per workspace at a time.
- Only the exec bit of a file is synced, not its full mode (files are 0644 or 0755).
- A command that exits with 125 is reported as a docker failure, since docker uses that code for its own errors.
- Nothing comes back but the log: no artifacts yet.
- No cache volumes, so each job starts cold (e.g. Go module and build cache).

Planned, in rough order: release binaries, deployment notes, then artifacts, cache volumes,
workspace cleanup.

## Development

```sh
make test    # go test -race ./...
make vet
make smoke   # end-to-end checks against a running server (scripts/)
make dist    # release binaries for linux amd64/arm64 + SHA256SUMS in dist/
```
