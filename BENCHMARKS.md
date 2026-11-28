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

## Node (pnpm)

yargs at `9588a76` as in [Node (npm)](#node-npm), set up as a pnpm project: `packageManager` set to
`pnpm@12.23.0`, `pnpm-lock.yaml` created with `pnpm import`, `storeDir` in `pnpm-workspace.yaml`,
and both files in `.prettierignore`. Command
`corepack pnpm install --frozen-lockfile && corepack pnpm test`, image `node:24`.

| Case | `caches` | Install time | Job time | Runs |
|---|---|---|---|---|
| No cache | none | 18.0–18.3s | 46.5–47.2s | 3 |
| First run with cache (fills it) | `/tmp/.cache`, `/tmp/.local/share/pnpm` | 28.0s | 51.9s | 1 |
| Warm | same | 4.2s | 27.3–27.7s | 3 |
| First run with `node_modules` cached | same and `/workspace/node_modules` | 4.2s | 27.2s | 1 |
| Warm | same | 0.17s | 23.2–23.6s | 3 |

- Without a cache, every job downloads pnpm through corepack as well as the packages.
- Filling the store costs about 10s once, on top of the uncached install.
- With `node_modules` up to date, pnpm skips the install and the `prepare` script; `pretest`
  still compiles, so the tests take the same 19.5s.
- `node_modules` is 130 MB per workspace. The shared caches are 114 MB of store, 89 MB of
  metadata and 74 MB of corepack's copy of pnpm.
- Measured in a fresh workspace: a `node_modules` cache left over from npm keeps npm's packages
  around (pnpm moves them aside as `.ignored_*`), so delete `.rwe/workspace` after switching.

## Bun

[Elysia](https://github.com/elysiajs/elysia) at `e212071`, a project developed on bun, with
`bun install --frozen-lockfile && bun run test:functionality && bun run test:types`, image
`oven/bun:1` (bun 1.4.5). That is `bun test` (1711 tests, 6.7s) and `tsc` (12.6s); Elysia's
`test:node` step needs npm and Node, which the image doesn't have. yargs isn't used here because
its test suite is written against Node's runtime.

| Case | `caches` | Install time | Job time | Runs |
|---|---|---|---|---|
| No cache | none | 0.9–1.1s | 25.5–26.2s | 3 |
| First run with cache (fills it) | `/tmp/.bun` | 0.8s | 25.0s | 1 |
| Warm | same | 0.18–0.20s | 24.2–24.5s | 3 |
| First run with `node_modules` cached | same and `/workspace/node_modules` | 0.2s | 24.3s | 1 |
| Warm | same | 0.01–0.3s | 23.3–23.9s | 3 |

- An empty job (`true`) takes 3.5s for this project, more than the 2.8s floor measured for Go.
- The install is a small part of the job with or without a cache: Elysia has 219 packages
  (135 MB in `node_modules`, the same in the cache).

## Rust

[clap](https://github.com/clap-rs/clap) at `f733b73`, `cargo test --locked`, image `rust:1`
(cargo 1.100). The root crate's tests pull in 66 crates from crates.io and build the project's own
three (`clap_lex`, `clap_builder`, `clap`).

| Case | `caches` | cargo time | Job time | Runs |
|---|---|---|---|---|
| No cache | none | 72.9–75.2s | 84.1–86.2s | 2 |
| First run with cache (fills it) | `/usr/local/cargo/registry`, `/usr/local/cargo/git` | 80.3s | 90.3s | 1 |
| Warm | same | 80.7–91.4s | 91.4–98.3s | 2 |
| First run with `target` cached | same and `/workspace/target` | 101.6s | 110.3s | 1 |
| Warm, no change | same | 6.7–6.9s | 16.1–16.2s | 2 |
| Warm, after an edit in `clap_builder` | same | 6.4–6.9s | 15.9–16.7s | 3 |

- The registry cache saves nothing measurable here: the 66 crates are 68 MB and download in a
  few seconds. It keeps jobs working when crates.io is slow or down.
- No change and an edit cost the same, because every job rebuilds the three local crates (see
  [the example](examples/rust/README.md)). Dependencies are not rebuilt.
- About 9s of every warm job is spent after the command exits, waiting for the ~500 MB of test
  binaries cargo rewrote to reach the disk. Writing 500 MB with `dd` shows the same: 0.4s inside
  the job, 9.9s for the job. The box's disk takes about 85 MB/s.
- The very first job took 146s, of which cargo was 91s; the rest was mostly pulling `rust:1`.
