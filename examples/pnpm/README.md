# Node (pnpm)

Copy [`.rwe.json`](.rwe.json) to the project root:

```json
{ "image": "node:24", "caches": ["/tmp/.cache", "/tmp/.local/share/pnpm", "/workspace/node_modules"] }
```

and point pnpm's store at the cache in `pnpm-workspace.yaml`:

```yaml
storeDir: /tmp/.local/share/pnpm/store
```

Then run tests with `rwe run -- sh -c 'corepack pnpm install --frozen-lockfile && corepack pnpm test'`.

| Cache | Holds | Why this path |
|---|---|---|
| `/tmp/.cache` | pnpm itself, downloaded by corepack, and pnpm's metadata cache | Jobs run with `HOME=/tmp`; shared by every job on the server |
| `/tmp/.local/share/pnpm` | The package store | Shared; the parent of `store`, so Docker doesn't create it as root |
| `/workspace/node_modules` | The installed dependencies | Kept per workspace, so an unchanged install takes under a second |

- `node:24` has no `pnpm` on the `PATH`, and a job can't run `corepack enable`, which writes to
  `/usr/local/bin`. `corepack pnpm` runs the version in `package.json`'s `packageManager` field.
- Without `storeDir`, pnpm puts the store inside `node_modules`, since the default store is on a
  different filesystem from the project. Then nothing is shared between workspaces.
- The store and `node_modules` are separate mounts, so pnpm copies files instead of hard-linking.
- Switching a project from npm: a `node_modules` cache left over from npm keeps npm's packages
  around (pnpm moves them aside as `.ignored_*`), so delete `.rwe/workspace` to start fresh.

## Results

[yargs](https://github.com/yargs/yargs)'s install and `npm test` goes from about 47s to 23.5s, of
which the tests themselves are 19.5s. Details in [BENCHMARKS.md](../../BENCHMARKS.md#node-pnpm).
