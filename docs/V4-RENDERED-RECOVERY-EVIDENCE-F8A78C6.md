# v4 rendered recovery evidence at tip f8a78c6

Direct Darwin Playwright Chromium allow → undo → fresh-process reload at
`f8a78c646877164009953401772acdb4d346175d` (`#551` behavior tip). Belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

`#551` changed `internal/ctxmgr/retention.go` and `internal/session/session.go`,
so prior behavior-SHA retention from `423d047…` is **invalid** for this tip.
This package rebinds Darwin rendered recovery at exact head; matching Linux and
Windows observations complete the tip three-platform aggregate.

## Provenance

- Fixture `vcs.revision=f8a78c6…`, `vcs.modified=false` (clean clone build;
  worktree builds from a dirty main checkout omit VCS stamps and fail closed)
- Browser: Playwright Chromium headless `151.0.7922.34` (arm64)
- Seed `2026-09-07T10:34:04.246371Z`; reload `2026-09-07T10:34:11.989225Z`
- Manifests: `source_sha_verified=true`, `source_tree_modified=false`
- Probe SHA-256 `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`

## Digests

```text
observation     69451fda862e36e9794802ef03448f1ba398cf9a98b008fcf3923a073fe80891
platform        7e3a0e1a916e096b92d216cf7b0f89ad9125ae279ad0566edec90bb3ee78e628
seed-manifest   742ab7d6b2209f5f2360fe2e0c249f468b8c3916e62ef6d9619a8e1846e527ba
reload-manifest 12845287f493a9dac82ba96fa97709c04b7ea673e2b7c7708810a19187397a99
```

Artifacts (outside checkout): `/private/tmp/picogent-evidence-f8a78c6/darwin/`.

Verdict `PASS`.

## Tip matrix (full cross-platform)

Exact-head matrix at clean `f8a78c6…` with Darwin platform + long-horizon +
three-platform aggregate:

- Digest `85847718b56306f38ef2f78cf5a7d5b425f28917429b458944ae2e9cce2f856d`
- Summary: **PASS 8 · INCONCLUSIVE 1 · UNVERIFIED 3**
- `rendered-platform-local=PASS`
- `rendered-cross-platform=PASS` (aggregate
  `5123c9cc1b024088c495322a56bee81462b3154949029d1887a65b6f76c708cd`)
- `rendered-long-horizon-local=PASS`
- Still **UNVERIFIED**: `live-provider-connectivity`, `live-provider-quality`,
  `hostile-filesystem-toctou`
- `release-authorization=INCONCLUSIVE`

Linux: [V4-RENDERED-LINUX-EVIDENCE-F8A78C6.md](V4-RENDERED-LINUX-EVIDENCE-F8A78C6.md).
Windows: [V4-RENDERED-WINDOWS-EVIDENCE-F8A78C6.md](V4-RENDERED-WINDOWS-EVIDENCE-F8A78C6.md).
Aggregate: [V4-RENDERED-CROSS-PLATFORM.md](V4-RENDERED-CROSS-PLATFORM.md).

Do **not** retain `--behavior-sha 423d047…` across `#551`.
