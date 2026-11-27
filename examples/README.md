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
{ "image": "golang:1.27" }
```

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
Both files accept `server`, `image` and `caches`; unknown keys are an error.

## Caches

`caches` lists container paths that are kept between jobs, so installs and builds start warm.
Jobs run as the server's user with `HOME=/tmp`, so tool caches live under `/tmp`:

```json
{ "image": "oven/bun:1", "caches": ["/tmp/.bun/install/cache", "/workspace/node_modules"] }
```

A path under `/workspace` is kept per workspace (one `node_modules` per project). Any other path
is shared by every job on the server, which suits download and build caches. They live in
`<data>/caches`; delete that directory to start cold again.

The server has no config file: `-addr` / `-data` flags, defaulting to `RWE_ADDR` / `RWE_DATA`,
then `:8999` / `local_data`. `RWE_JOB_MEMORY` (or `-job-memory`, e.g. `2g`) caps each job's memory:
it is the default for jobs that set none, and a job asking for more is rejected. Unset means no cap.
`RWE_LOG_RETENTION` (or `-log-retention`, default `168h`) deletes a job's log that long after it
finished; its status stays available. `0` keeps logs forever.
Under systemd, load the env file with
`EnvironmentFile=/etc/rwe/server.env`; `systemctl restart rwe` picks up changes.
