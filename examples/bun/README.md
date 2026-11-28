# Bun

Copy [`.rwe.json`](.rwe.json) to the project root:

```json
{ "image": "oven/bun:1", "caches": ["/tmp/.bun", "/workspace/node_modules"] }
```

Then run tests with `rwe run -- sh -c 'bun install --frozen-lockfile && bun test'`.

| Cache | Holds | Why this path |
|---|---|---|
| `/tmp/.bun` | Downloaded packages (`/tmp/.bun/install/cache`) | Jobs run with `HOME=/tmp`; shared by every bun job on the server |
| `/workspace/node_modules` | The installed dependencies | Kept per workspace, so an unchanged install is a no-op |

The image has no npm, and `node` in it is bun. Scripts that call `npm` or need Node itself need a
different image.

## Results

[Elysia](https://github.com/elysiajs/elysia)'s install takes under a second even without a cache,
so the caches save about a second per job. Details in
[BENCHMARKS.md](../../BENCHMARKS.md#bun).
