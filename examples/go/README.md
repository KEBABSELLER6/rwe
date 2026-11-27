# Go

Copy [`.rwe.json`](.rwe.json) to the project root:

```json
{ "image": "golang:1.27", "caches": ["/tmp/.cache/go-build", "/go/pkg"] }
```

| Cache | Holds | Why this path |
|---|---|---|
| `/tmp/.cache/go-build` | Compiled packages and test results (`GOCACHE`) | Jobs run with `HOME=/tmp` |
| `/go/pkg` | Downloaded modules (`/go/pkg/mod`) and checksum database state (`/go/pkg/sumdb`) | Caching only `/go/pkg/mod` leaves `/go/pkg` owned by root, and `go get` fails writing `sumdb` |

Both are shared by every Go job on the server. Go guards its caches against concurrent use and
trims build cache entries unused for 5 days.

A project without dependencies only needs the build cache; this repo's own
[`.rwe.json`](../../.rwe.json) has just `/tmp/.cache/go-build`.

## Results

`make test` (`go test -race ./...`) on this repo goes from about 42s to 5–8s once the build cache is
warm. Details in [BENCHMARKS.md](../../BENCHMARKS.md#go).
