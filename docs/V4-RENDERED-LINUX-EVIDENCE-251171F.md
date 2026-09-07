# v4 Linux rendered recovery evidence at tip 251171f

This record refreshes the direct Linux owned-browser observation under parent
[#453](https://github.com/saiaathish/picogent/issues/453) at exact behavior tip
`251171f2972cc3a28b03cf35c222e77134cb65b4`.

It proves only the `linux/arm64` rendered recovery path. Tip-bound Windows was
not recollected, so tip `rendered-cross-platform` remains `UNVERIFIED` (the
historical aggregate PASS stays exact-candidate bound to `18dc4a1…`). This
record does not upgrade hostile-filesystem TOCTOU or release authorization.

## Provenance

- Source: fresh detached clone inside a task-owned Linux container.
- Fixture binary:
  `vcs.revision=251171f2972cc3a28b03cf35c222e77134cb65b4`,
  `vcs.modified=false`.
- Runtime: Docker Desktop Linux VM, `linux/arm64`; Go `1.25.14`.
- Owned browser: Chromium `152.0.7977.82`, headless mode, with a disposable
  task-owned profile in the same Linux container.
- Fixture: `go-build-tags-rendered_fixture`; session
  `rendered-recovery-fixture`.
- Seed started at `2026-09-07T04:11:34.906304425Z`; reload started in a fresh
  fixture process at `2026-09-07T04:11:49.819134335Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`.
- Probe content SHA-256:
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.

The source clone, compiled fixture, fixture home, workspace, browser profile,
and browser process were task-owned and disposable. Chromium drove the normal
embedded GUI and its real permission, chat, undo, task-store, and session-store
paths. No hosted fixture/API test was used as browser evidence.

## Direct rendered observations

1. The seed page rendered in Safe mode without Undo; the probe was absent.
2. After prompt submission, Chromium rendered all four permission controls
   while the probe remained absent.
3. Allow rendered `Edited 1 file`, `Changed files (1)`, and
   `Undo last change`; the probe digest matched the fixture contract.
4. Undo removed the probe and made Undo unavailable while
   `Changed files (1)` remained visible.
5. After the seed process stopped, the same owned Chromium process loaded a
   fresh reload process and rendered durable `Changed files (1)` without stale
   Undo; the probe remained absent.

The observation verdict is `PASS`.

## Host-local artifacts

Artifacts remain outside the checkout under:

```text
/private/tmp/picogent-evidence-251171f/linux/
```

Artifact SHA-256:

```text
observation       fe88fa539488d0616c78c2ba28d145d98ac32c4ed2f6af74acef0b7041c60a00
platform          4a2c71964549fafc54627bd7308cdb95408cca045a51e4aedd669ce1ebafdeaf
seed-manifest     6d8dbe402e72ed2bc33e9d4510775659d84118d66198729162e2a2c9e167d313
reload-manifest   79160afd3f7f7e4ee467978d3a0fe2498f6d9df64bad4fcdfaa26bf27c40ff07
screenshot-set    649a89c3b9dbbcf2cb57dfdf556a9d13fa77e5d91760e746b7f89aaf1d9fdc85
initial           98aff12813ef3bfb3e0adc085f49eee42badb1bb70d41f6e09f6515e01f0732b
permission        0c739c24e391cedb8256d07a38e24bc2945aa01b371313ba1aea9b1bddbed202
allowed           8dea7ecac9b1cb2e2459203ac9450ea78c352ff489159966a5039c920feebedb
undone            6edc9a5fcb5c2b3fc6f17c39f86ee3bf7ecfd8b544b0bdb3c291829a6c4383f2
reload            7154ac207ff8d34b72e24870118e309c6e7046a9dd55c35ce1f9c92019dbc858
```

The digest-only platform record contains:

```text
candidate_sha=251171f2972cc3a28b03cf35c222e77134cb65b4
platform=linux
architecture=aarch64
environment=task-owned-disposable
browser=chromium-headless-152.0.7977.82
fixture=rendered-recovery
observation_sha256=fe88fa539488d0616c78c2ba28d145d98ac32c4ed2f6af74acef0b7041c60a00
screenshot_sha256=649a89c3b9dbbcf2cb57dfdf556a9d13fa77e5d91760e746b7f89aaf1d9fdc85
verdict=PASS
source_tree_modified=false
```

Matching Darwin and Linux tip artifacts agree on exact SHA
`251171f2972cc3a28b03cf35c222e77134cb65b4`. Tip Windows was not recollected, so
no tip three-platform aggregate was produced.
