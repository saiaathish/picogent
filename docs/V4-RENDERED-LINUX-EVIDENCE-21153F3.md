# v4 Linux rendered recovery evidence at tip 21153f3

This record refreshes the direct Linux owned-browser observation under parent
[#453](https://github.com/saiaathish/picogent/issues/453) at exact tip SHA
`21153f3a5626e727a5b29f9cacb351fcf4ae4e7f`.

It proves only the `linux/arm64` rendered recovery path. Tip-bound Windows was
not recollected, so tip `rendered-cross-platform` remains `UNVERIFIED` (the
historical aggregate PASS stays exact-candidate bound to `18dc4a1…`). This
record does not upgrade hostile-filesystem TOCTOU or release authorization.

## Provenance

- Source: fresh detached clone inside a task-owned Linux container.
- Fixture binary:
  `vcs.revision=21153f3a5626e727a5b29f9cacb351fcf4ae4e7f`,
  `vcs.modified=false`.
- Runtime: Docker Desktop Linux VM, `linux/arm64`; Go `1.25.14`.
- Owned browser: Chromium `152.0.7977.82`, headless mode, with a disposable
  task-owned profile in the same Linux container.
- Fixture: `go-build-tags-rendered_fixture`; session
  `rendered-recovery-fixture`.
- Seed started at `2026-09-07T03:55:46.453412088Z`; reload started in a fresh
  fixture process at `2026-09-07T03:55:56.777031593Z`.
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
/private/tmp/picogent-evidence-21153f3/linux/
```

Artifact SHA-256:

```text
observation       73e3c378c1c4cc6a6dac714c334cffd5229edf991e31366355dc7aff372f4e46
platform          09c4587c8d0c71d2bdf3133883d9cf1daec9ab4fbb116b7c6ff71386717eb466
seed-manifest     b2f46237518e9205d52c98821e0531de3b2965468757c6e6e46a45326fdb2d65
reload-manifest   f23cf2ac7f25aae7ce8fcd83017726a9e181e8d0e00303be0b86c4c948699894
screenshot-set    8dbfd4bb872ade8fdde659e45c0b51904777d8ad5a730d72ebd5317244e072d5
initial           ebca7d257401e07833116a84e4b8f0811aa360029719fe8c471530bf48c38965
permission        0c739c24e391cedb8256d07a38e24bc2945aa01b371313ba1aea9b1bddbed202
allowed           b0ce8d6eda9a5605c9e681f4ab8ff18f2ed399765d3d9bb831abc18878d6cd43
undone            baddefd656a6f316ec84be0d95943972096024b487f2e4964c43469969c7ab47
reload            e7cfb18d5d8302719c9fe64fee75f373fc6566c4f783d5e683a602c867235ec4
```

The digest-only platform record contains:

```text
candidate_sha=21153f3a5626e727a5b29f9cacb351fcf4ae4e7f
platform=linux
architecture=aarch64
environment=task-owned-disposable
browser=chromium-headless-152.0.7977.82
fixture=rendered-recovery
observation_sha256=73e3c378c1c4cc6a6dac714c334cffd5229edf991e31366355dc7aff372f4e46
screenshot_sha256=8dbfd4bb872ade8fdde659e45c0b51904777d8ad5a730d72ebd5317244e072d5
verdict=PASS
source_tree_modified=false
```

Matching Darwin and Linux tip artifacts agree on exact SHA
`21153f3a5626e727a5b29f9cacb351fcf4ae4e7f`. Tip Windows was not recollected, so
no tip three-platform aggregate was produced.
