# Python (uv)

Copy [`.rwe.json`](.rwe.json) to the project root:

```json
{ "image": "ghcr.io/astral-sh/uv:python3.13-bookworm-slim", "caches": ["/tmp/.cache/uv", "/workspace/.venv"] }
```

Then run tests with `rwe run -- uv run pytest`.

| Cache | Holds | Why this path |
|---|---|---|
| `/tmp/.cache/uv` | Downloaded and built packages | Jobs run with `HOME=/tmp`; shared by every uv job on the server |
| `/workspace/.venv` | The project's virtualenv | Kept per workspace, so `uv run` only checks it against `uv.lock` |

uv prints `Failed to hardlink files; falling back to full copy`: the cache and the virtualenv are
separate mounts, and hard links can't cross them. The copy works the same way. To silence it, add
`link-mode = "copy"` under `[tool.uv]` in `pyproject.toml`.

The slim image has no `less`, so tests that start a pager fail.

## Results

[click](https://github.com/pallets/click)'s test suite goes from about 20s to 14.7s, of which
pytest itself is 8.5s. Details in [BENCHMARKS.md](../../BENCHMARKS.md#python-uv).
