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
- cargo decides what to rebuild by file time. rwe keeps file times when it copies the workspace for a
  job (server v0.4.0 and later), so only the crates you changed are rebuilt.

## Results

[clap](https://github.com/clap-rs/clap)'s `cargo test` goes from about 85s to 4–7s once `target` is
warm, and 16–18s after an edit in `clap_builder`. Details in [BENCHMARKS.md](../../BENCHMARKS.md#rust).
