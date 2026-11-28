# Node (npm)

Copy [`.rwe.json`](.rwe.json) to the project root:

```json
{ "image": "node:24", "caches": ["/tmp/.npm", "/workspace/node_modules"] }
```

Then run tests with `rwe run -- sh -c 'npm install && npm test'`.

| Cache | Holds | Why this path |
|---|---|---|
| `/tmp/.npm` | Downloaded package tarballs | Jobs run with `HOME=/tmp`; shared by every npm job on the server |
| `/workspace/node_modules` | The installed dependencies | Kept per workspace, so `npm install` only checks it against `package-lock.json` |

`npm ci` deletes `node_modules` before installing, so with `npm ci` only the npm cache helps. It
still works with `node_modules` cached: npm empties the directory instead of removing it.

## Results

[yargs](https://github.com/yargs/yargs)'s `npm test` with the install goes from about 31s to 26s,
of which the tests themselves are 18s. Details in [BENCHMARKS.md](../../BENCHMARKS.md#node-npm).
