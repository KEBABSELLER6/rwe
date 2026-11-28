# Using rwe from a project

This repo verifies itself with rwe: its own [`.rwe.json`](../.rwe.json) picks the image, and
`rwe run -- make test` from the repo root runs the test suite on the server.
The files here are the parts that differ per machine or per project.

| File | Copy to | Purpose |
|---|---|---|
| [`config.json`](config.json) | `~/.config/rwe/config.json` | Which server to talk to |
| [`.mcp.json`](.mcp.json) | `<project>/.mcp.json` | Registers `rwe mcp` with Claude Code for that project |
| [`AGENTS.md`](AGENTS.md) | paste into `<project>/CLAUDE.md` or `AGENTS.md` | Tells the agent to verify remotely instead of locally |
| [`server.env`](server.env) | `/etc/rwe/server.env` on the box | Server listen address and data directory |

Per project you also want a `.rwe.json`, like the one at this repo's root:

```json
{ "image": "golang:1.27", "caches": ["/tmp/.cache/go-build"] }
```

See [Caches](#caches) for ready-made ones per stack.

## Setup

1. Start the server on the box: `make build` and run `bin/server` (needs Docker).
2. Install the client on your machine: `go install ./cmd/rwe` from this repo, or put `bin/rwe` on your `PATH`.
3. Write `~/.config/rwe/config.json` with the server address.
4. Check it by verifying this repo: `rwe run -- make test` (exit code is the job's).
5. In the project you want to verify, add `.rwe.json`, `.mcp.json` and the `AGENTS.md` snippet.
   Instead of `.mcp.json` you can register it for yourself only: `claude mcp add rwe -- rwe mcp`.

## Config precedence

Later wins: built-in defaults (`http://localhost:8999`, `golang:1.27`) →
`~/.config/rwe/config.json` → `<project>/.rwe.json` → `RWE_URL` / `RWE_IMAGE` → `rwe run -i IMAGE`.
Both files accept `server`, `image`, `caches` and `artifacts`; unknown keys are an error.

The server has no config file: `-addr` / `-data` flags, defaulting to `RWE_ADDR` / `RWE_DATA`,
then `:8999` / `local_data`. `RWE_JOB_MEMORY` (or `-job-memory`, e.g. `2g`) caps each job's memory:
it is the default for jobs that set none, and a job asking for more is rejected. Unset means no cap.
`RWE_LOG_RETENTION` (or `-log-retention`, default `168h`) deletes a job's log that long after it
finished; its status stays available. `0` keeps logs forever.
`RWE_WORKSPACE_RETENTION` (or `-workspace-retention`, default `720h`) deletes a workspace that long
after its last sync or job; the next `rwe run` uploads a fresh one. `0` keeps workspaces forever.
`RWE_CACHE_RETENTION` (or `-cache-retention`, default `720h`) deletes a shared cache that long after
the last job that used it. A workspace's own caches are deleted together with the workspace.
`RWE_ARTIFACT_RETENTION` (or `-artifact-retention`, default `24h`) deletes a job's artifacts that long
after it finished. `RWE_ARTIFACT_MAX_SIZE` (or `-artifact-max-size`, default `256m`) drops a job's
artifacts when together they are larger; the job's status is unaffected. `0` turns either off.
Under systemd, load the env file with
`EnvironmentFile=/etc/rwe/server.env`; `systemctl restart rwe` picks up changes.

## Caches

`caches` lists container paths that are kept between jobs, so installs and builds start warm.
Jobs run as the server's user with `HOME=/tmp`, so tool caches usually live under `/tmp`.

- A path under `/workspace` is kept per workspace, e.g. `/workspace/node_modules`.
- Any other path is shared by every job on the server, which suits download and build caches.
- Docker creates missing parent directories of a cache path as root, and the job can't write
  there. If a tool also writes next to its cache, cache the parent instead (Go: `/go/pkg`, not
  `/go/pkg/mod`).
- They live in `<data>/caches` on the box; delete that directory to start cold again. Unused caches
  are deleted on their own, see `RWE_CACHE_RETENTION` above.

Tested setups, with timings in [BENCHMARKS.md](../BENCHMARKS.md):

| Stack | Example |
|---|---|
| Go | [`go/`](go/) |
| Python (uv) | [`python/`](python/) |
| Node (npm) | [`node/`](node/) |
| Node (pnpm) | [`pnpm/`](pnpm/) |
| Bun | [`bun/`](bun/) |
| Rust | [`rust/`](rust/) |

## Artifacts

`artifacts` lists files or directories, relative to the project root, that come back after a job:

```json
{ "image": "golang:1.27", "artifacts": ["coverage.out", "dist"] }
```

- They are collected whatever the job's status, so a failed run's test reports come back too.
- `rwe run -a PATH` (repeatable) and the MCP `run` tool's `artifacts` argument replace the list for one job.
- They land in `.rwe/artifacts/<job id>/`, keeping their paths. Each download replaces the previous
  job's, so only the latest are kept.
- A path the job didn't create is noted in the job log, not an error. Paths inside a workspace
  cache (e.g. `node_modules`) don't come back.
