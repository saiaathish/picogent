# v4 Linux rendered recovery evidence at tip 423d047

Direct Linux Chromium owned-browser observation at
`423d0471c1864651473c2f1828b67066885f79bd` under
[#453](https://github.com/saiaathish/picogent/issues/453).

## Provenance

- Chromium `152.0.7977.82` headless in task-owned Linux container
- Seed `2026-09-07T05:50:09.030634299Z`; reload `2026-09-07T05:50:18.687847428Z`
- Manifests: `source_sha_verified=true`, `source_tree_modified=false`

## Digests

```text
observation       1fb6937e4ce901579eb4a4fcb53e8ecd87959c043ff634fea2bf976aa4fb8409
platform          4452a8b9f21f282519ca1b8e95aca02c11db03f6b02723b2112c69017c53352a
seed-manifest     48bef5d713ce668f81c36cb6965f4296a8fb70e636fa07132185896bd10eea42
reload-manifest   1d224821ffcde39a88ee02cde78edbd00769739b6266f9305d5028760b1eb4d3
screenshot-set    f355910a95119d81e27b56da8b0143eed7d3a73a3c041e938451a1c95ec10de5
initial           d7b78e6c7d011eb4dcc15a020be4b5319f04d4ee223d620f460168b99cc3e221
permission        0c739c24e391cedb8256d07a38e24bc2945aa01b371313ba1aea9b1bddbed202
allowed           8dea7ecac9b1cb2e2459203ac9450ea78c352ff489159966a5039c920feebedb
undone            b9c1ddd34c4ef6bc036a81ffbe4e65d1cabb69d1df412a66ff399a68ca29c983
reload            28a14ac00f20b2c0d79f3ed817431266085b688761989853fb16cdfcd638536a
```

Verdict `PASS`. Tip Windows recollection and three-platform aggregate are
recorded in [V4-RENDERED-WINDOWS-EVIDENCE-423D047.md](V4-RENDERED-WINDOWS-EVIDENCE-423D047.md)
and [V4-RENDERED-CROSS-PLATFORM.md](V4-RENDERED-CROSS-PLATFORM.md); exact-SHA
matrix projection is `rendered-cross-platform=PASS` when that aggregate is
supplied. Retain across docs-only descendants with
`--behavior-sha 423d0471c1864651473c2f1828b67066885f79bd`.
