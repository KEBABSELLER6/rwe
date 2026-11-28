# Benchmarks

How long a job takes with and without [caches](examples/README.md#caches).

## Setup

| | |
|---|---|
| Box | Intel Core i3-4130T (2 cores, 4 threads, 2.9 GHz), 7 GB RAM |
| Server | rwe v0.3.0 |
| Measured | 2026-11-27, with `rwe run` from a laptop on the same Tailscale network |

Times are the job's duration as reported by `rwe run` (`==> passed in …`). The client's wall-clock
time was within 0.4s of it in every run.

A job that does almost nothing (`go env`) takes 2.8s: syncing, starting the container and removing
it. That is the floor for every row below.

## Go

This repo, `make test` (`go test -race ./...`), image `golang:1.27` (Go 1.27.4). It has no module
dependencies, so only the build cache matters.

| Case | `caches` | Job time | Runs |
|---|---|---|---|
| No cache | none | 41.4–41.7s | 3 |
| First run with cache (fills it) | `/tmp/.cache/go-build` | 41.7s | 1 |
| Warm, after an edit in `internal/service` | same | 4.9–7.5s | 3 |
| Warm, all tests rerun (`go test -race -count=1 ./...`) | same | 7.3–7.7s | 3 |
| Warm, no change (Go reuses earlier test results) | same | 2.8s | 3 |

- The ~38s without a cache is Go compiling the standard library and the repo with `-race`.
  Filling the cache costs nothing extra on the first run.
- The edit row is the usual agent loop: the changed package and the packages that import it are
  rebuilt and their tests rerun.
- The no-change row mostly shows rwe's 2.8s floor, since no test actually runs.

Module downloads, checked separately with `go get golang.org/x/text@v0.21.0` in an empty module and
`/go/pkg` cached: 6.7s on the first run, 3.0s on the second with nothing downloaded.

## Python (uv)

[click](https://github.com/pallets/click) at `08b803c`, with
`uv run pytest -q --ignore=tests/test_utils/test_echo_via_pager.py` (the ignored tests need `less`,
which the image lacks), image `ghcr.io/astral-sh/uv:python3.13-bookworm-slim` (uv 0.9.30,
Python 3.13). `uv run` installs the project and its default dependency groups (pytest, mypy,
pyright, ruff, tox and others) before running about 2,600 tests.

| Case | `caches` | Job time | Runs |
|---|---|---|---|
| No cache | none | 19.8–22.1s | 3 |
| First run with cache (fills it) | `/tmp/.cache/uv` | 19.1s | 1 |
| Warm | same | 16.4–16.7s | 3 |
| First run with venv cached | `/tmp/.cache/uv`, `/workspace/.venv` | 16.2s | 1 |
| Warm | same | 14.6–14.9s | 3 |

- pytest reports 8.5–11s in every run, so the uncached install costs about 8s on top of the 2.8s
  floor, and both caches together bring it down to about 3.5s.
- With only the uv cache, uv still builds a fresh virtualenv and copies every package into it.
  Caching `.venv` skips that.
- click's dependencies are small. A project with large packages (numpy, torch) should gain more
  from the uv cache, since downloads would dominate; not measured here.

## Node (npm)

[yargs](https://github.com/yargs/yargs) at `9588a76`, a TypeScript project, with
`npm ci && npm test` or `npm install && npm test`, image `node:24`. `npm test` compiles with `tsc`,
runs mocha under c8 coverage, then eslint and prettier. The install step also runs `tsc` through
yargs's `prepare` script, which takes 3.3s whatever is cached.

| Case | Install | `caches` | Install time | Job time | Runs |
|---|---|---|---|---|---|
| No cache | `npm ci` | none | 8.9s | 30.8–32.8s | 2 |
| First run with cache (fills it) | `npm ci` | `/tmp/.npm` | 8.6s | 30.5s | 1 |
| Warm | `npm ci` | same | 6.8–7.2s | 29.2–32.0s | 3 |
| First run with `node_modules` cached | `npm install` | `/tmp/.npm`, `/workspace/node_modules` | 6.7s | 28.8s | 1 |
| Warm | `npm install` | same | 4.6–4.7s | 26.1–26.4s | 3 |
| Warm, `npm ci` | `npm ci` | same | 6.9s | 28.7s | 1 |

- `npm test` takes 18s in every run (one outlier at 20.7s), so the caches only change the install.
- Without the `prepare` script, npm's own share is about 5.6s with no cache, 3.5s with the npm
  cache and 1.2s with `node_modules` kept as well.
- `node_modules` is 128 MB per workspace for yargs; the npm cache is 27 MB.
- The very first job took 72.3s because the box was pulling `node:24`; that happens once per
  image and is left out above.
