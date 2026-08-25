# V4 repo provenance

`internal/repomap.Capture` adds a fresh, bounded provenance snapshot to the
existing on-demand repository map. It does not persist state or introduce a
watcher, cache, index, daemon, or multi-root service.

Each snapshot records:

- the absolute workspace root and detected Git root;
- the full immutable `HEAD` object ID when a committed Git head is available;
- normalized dirty paths scoped to the requested workspace;
- bounded, sorted manifest paths, including nested manifests;
- explicit unknown/truncation state so unavailable or incomplete Git evidence
  cannot be rendered as clean.

The existing map fields remain at the top level of `repo_map` output. Exact
provenance is additive under `provenance`. `repomap.Inspect` and
`repomap.Format` remain available for compatibility.

## Local benchmark evidence

Command:

```sh
go test ./internal/benchmark -run '^$' -bench 'BenchmarkRepoMap(Capture|SnapshotFormat)$' -benchmem -count=3
```

On 2026-08-25, Apple M3 arm64 macOS, the non-Git nested-manifest fixture
measured:

| Operation | Observed range | Allocation range |
| --- | ---: | ---: |
| Snapshot capture | 30.1–32.3 ms/op | 70.7–71.6 KB, 429–432 allocs/op |
| Snapshot formatting | 4.1–4.3 µs/op | 3,701 B, 13 allocs/op |

These are local deterministic measurements. They do not claim live-provider,
GUI, browser, or end-to-end task quality.
