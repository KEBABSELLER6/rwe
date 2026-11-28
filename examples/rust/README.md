# Rust

Copy [`.rwe.json`](.rwe.json) to the project root:

```json
{ "image": "rust:1", "caches": ["/usr/local/cargo/registry", "/usr/local/cargo/git", "/workspace/target"] }
```

Then run tests with `rwe run -- cargo test`.

| Cache | Holds | Why this path |
|---|---|---|
| `/usr/local/cargo/registry` | Downloaded crates and their unpacked sources | `CARGO_HOME` is `/usr/local/cargo` in the image; shared by every Rust job |
| `/usr/local/cargo/git` | Git dependencies | Same; empty for a project with none |
| `/workspace/target` | Compiled dependencies, test binaries, incremental state | Kept per workspace; this is where the time goes |

- Don't cache all of `/usr/local/cargo`: it also holds `cargo` and `rustc` themselves.
- `target` is large (1.4 GB for clap) and nothing cleans it up yet. Delete
  `<data>/caches/workspaces` on the box to reclaim it.
- Every job rebuilds the project's own crates, even with no change: rwe copies the workspace for
  each job without keeping file times, and cargo decides by file time. Dependencies stay built.

## Results

[clap](https://github.com/clap-rs/clap)'s `cargo test` goes from about 85s to 16s once `target` is
warm. Details in [BENCHMARKS.md](../../BENCHMARKS.md#rust).
