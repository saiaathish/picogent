# v4 rendered recovery evidence at tip 423d047

Direct Darwin Playwright Chromium allow → undo → fresh-process reload at
`423d0471c1864651473c2f1828b67066885f79bd`. Belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

## Provenance

- Fixture `vcs.revision=423d047…`, `vcs.modified=false`
- Browser: Playwright Chromium headless `134.0.6998.35` (arm64)
- Seed `2026-09-07T05:52:00.810035Z`; reload `2026-09-07T05:52:08.938831Z`
- Manifests: `source_sha_verified=true`, `source_tree_modified=false`
- Probe SHA-256 `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`

## Digests

```text
observation     0771e752083183e89af8f5e0787a5c72bdfa9d67922452746d04c453e5885bcc
platform        5ca9580a5d39779c493a5b114b8a7fc689b8a9d97f475a3856b2ffc602a1e56a
seed-manifest   ec59071c70d3977b3398fc2d97ed64da0ae12f54d3a649552879a750ad3aa75d
reload-manifest c62144561940df0338312cc2d5b7d8323b3b6ac6a33ba7af5eaeb65e8c8f33f2
```

Verdict `PASS`. Tip Windows recollection and three-platform aggregate are
recorded in [V4-RENDERED-WINDOWS-EVIDENCE-423D047.md](V4-RENDERED-WINDOWS-EVIDENCE-423D047.md)
and [V4-RENDERED-CROSS-PLATFORM.md](V4-RENDERED-CROSS-PLATFORM.md); exact-SHA
matrix projection is `rendered-cross-platform=PASS` when that aggregate is
supplied. Retain across docs-only descendants with
`--behavior-sha 423d0471c1864651473c2f1828b67066885f79bd`.
