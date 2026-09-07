# v4 Linux rendered recovery evidence at tip f8a78c6

Direct Linux Chromium owned-browser observation at
`f8a78c646877164009953401772acdb4d346175d` under
[#453](https://github.com/saiaathish/picogent/issues/453).

## Provenance

- Chromium `152.0.7977.82` headless in task-owned Linux container
  (`picogent-rendered-linux-507:local` / golang bookworm path; PATH includes
  `/usr/local/go/bin`)
- Clean detached checkout at `f8a78c6…`; fixture
  `vcs.revision=f8a78c646877164009953401772acdb4d346175d`,
  `vcs.modified=false`
- Seed `2026-09-07T10:50:43.11818026Z`; reload `2026-09-07T10:50:52.923544625Z`
- Manifests: `source_sha_verified=true`, `source_tree_modified=false`
- Probe SHA-256 `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`

## Digests

```text
observation       7b9bb7b186c7e17174ec91a3dbd6fd146bd60ee071a327770f1e306d3ffa473e
platform          69928933990067222ad39c4dd68e4986342b757ec8f4a5175b49585184863053
seed-manifest     46102dc19d9164c00f4cbbae4847055212065bd3366b5be951816516e4da660b
reload-manifest   5d396f926e924750561742046b0931e1e66716c193517f1476a530cc275f12e0
screenshot-set    b46f51fe2ff4013f135b585c0e96e0688b28352c4fad1c3f73de45a4ddf5bcf0
initial           00dbb859ca772c709819ce6648610ead083e97b8c8b938d762816ea7f3beba82
permission        f11bc799c816d58895147f6c66d41c4932d9f2d600aaf152461b7c8bcf56cd5a
allowed           32ea6a5eff400df6f0914a480e0ef316481a6415d9c2cc8c295b9e9f161c3f4e
undone            a75dffc604d261190f8d1616e73e77fe6669801e92d1c6adb93196f3b8bf8410
reload            0a16d6d629a54d1068e5c38f1341eb89f79132f3d5abe6e9bf1dfdde1c8b87a2
```

Artifacts (outside checkout): `/private/tmp/picogent-evidence-f8a78c6/linux/`.

Verdict `PASS`. Tip Windows recollection and three-platform aggregate are
recorded in [V4-RENDERED-WINDOWS-EVIDENCE-F8A78C6.md](V4-RENDERED-WINDOWS-EVIDENCE-F8A78C6.md)
and [V4-RENDERED-CROSS-PLATFORM.md](V4-RENDERED-CROSS-PLATFORM.md); exact-SHA
matrix projection is `rendered-cross-platform=PASS` when that aggregate is
supplied. Retain across docs-only descendants with
`--behavior-sha f8a78c646877164009953401772acdb4d346175d`.
