# v4 Linux rendered recovery evidence at tip 18dc4a1

This record refreshes the direct Linux owned-browser observation required by
[#507](https://github.com/saiaathish/picogent/issues/507), under parent
[#453](https://github.com/saiaathish/picogent/issues/453), at exact
behavior SHA `18dc4a1ca1137ab78dfd0102848eb99c417653cc`.

It proves only the `linux/arm64` rendered recovery path. Windows remains
unobserved, so `rendered-cross-platform` remains `UNVERIFIED`; no aggregate
artifact was produced. This record does not upgrade hostile-filesystem TOCTOU
or release authorization.

## Provenance

- Source: fresh detached clone inside a task-owned Linux container.
- Fixture binary:
  `vcs.revision=18dc4a1ca1137ab78dfd0102848eb99c417653cc`,
  `vcs.modified=false`.
- Runtime: Docker Desktop Linux VM, `linux/arm64`; Go `1.25.14`.
- Owned browser: Chromium `152.0.7977.82`, headless mode, with a disposable
  task-owned profile in the same Linux container.
- Fixture: `go-build-tags-rendered_fixture`; session
  `rendered-recovery-fixture`.
- Seed started at `2026-09-07T02:07:08.331816838Z`; reload started in a fresh
  fixture process at `2026-09-07T02:07:17.717013051Z`.
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
/private/tmp/picogent-evidence-post519/linux/
```

Artifact SHA-256:

```text
observation       9ccfbf51d8f6f7f54c3c71d8eb863373628a017789a63c194de7f991e9b79f4a
platform          e506dbd517e32fdf5e058064a12d8db8d8f1fea51a2babd0fc708718b70f32b4
seed-manifest     7f454e334a59879173604506de3117f0ab1380123633f4bc1ba8d567c74392a6
reload-manifest   5e07a6be34b57b4a540939c1ff05eb6ca315e4e2ecb7d9104138442cf1113dc6
screenshot-set    71da05606c4e04d5170ff523e2068be1b5876c26bf5fbaf90d864b3aad61ecc0
initial           f9401d6cee30c6d2f0cf2d53c8c813e3121c58fd55de38a7858991af75f8d5d2
permission        0c739c24e391cedb8256d07a38e24bc2945aa01b371313ba1aea9b1bddbed202
allowed           2c375a9ca4ec37923076f2d1004ab61b0aece29db6920f03769265f888ed35b4
undone            4b389c45953f5d8a3986c92c6027ec8ee4228e4134a580b028d098d2c9169f28
reload            7f9204f17c931b6a41c9c3b76c35858f0d1141d792819e0238e92dc5449e679a
```

The digest-only platform record contains:

```text
candidate_sha=18dc4a1ca1137ab78dfd0102848eb99c417653cc
platform=linux
architecture=aarch64
environment=task-owned-disposable
browser=chromium-headless-152.0.7977.82
fixture=rendered-recovery
observation_sha256=9ccfbf51d8f6f7f54c3c71d8eb863373628a017789a63c194de7f991e9b79f4a
screenshot_sha256=71da05606c4e04d5170ff523e2068be1b5876c26bf5fbaf90d864b3aad61ecc0
verdict=PASS
source_tree_modified=false
```

The matching Darwin and Linux artifacts agree on exact behavior SHA
`18dc4a1ca1137ab78dfd0102848eb99c417653cc`. A real Windows owned-browser
artifact at that exact SHA is still required before the three-input packager
can run and the cross-platform row can become `PASS`.
