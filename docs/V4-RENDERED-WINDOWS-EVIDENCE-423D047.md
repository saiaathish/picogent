# v4 Windows rendered recovery evidence at tip 423d047

This record adds the direct Windows owned-browser observation at exact behavior
SHA `423d0471c1864651473c2f1828b67066885f79bd` under parent
[#453](https://github.com/saiaathish/picogent/issues/453).

It proves the `windows/amd64` rendered recovery path in a task-owned disposable
Playwright Chromium profile on GitHub Actions `windows-latest`. Combined with
the matching Darwin and Linux observations at the same SHA, the three-input
packager produced a tip PASS aggregate. This record does not upgrade
hostile-filesystem TOCTOU or release authorization.

## Provenance

- Source: clean detached checkout at
  `423d0471c1864651473c2f1828b67066885f79bd` inside the hosted Windows runner.
- Fixture binary:
  `vcs.revision=423d0471c1864651473c2f1828b67066885f79bd`,
  `vcs.modified=false` (verified via `go version -m` / fixture-buildinfo).
- Runtime: GitHub Actions `windows-latest`, Go `1.25.x`.
- Owned browser: Playwright Chromium `134.0.6998.35`, headless, disposable
  persistent profile under Local Temp.
- Fixture: `go-build-tags-rendered_fixture`; session
  `rendered-recovery-fixture`.
- Seed started at `2026-09-07T06:26:25.7362819Z`; reload started in a fresh
  fixture process at `2026-09-07T06:26:31.3206005Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`.
- Probe content SHA-256:
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.
- Collection run:
  [34090755069](https://github.com/saiaathish/picogent/actions/runs/34090755069)
  (artifact
  `rendered-windows-owned-browser-423d0471c1864651473c2f1828b67066885f79bd`,
  zip digest
  `sha256:4750b1ae1685aa1d208b65a55d46a0a6a947d2dc812942ef58a0bd0095d3f321`).

The source checkout, compiled fixture, fixture home, workspace, browser
profile, and browser process were task-owned and disposable. Chromium drove the
normal embedded GUI permission, chat, undo, task-store, and session-store
paths. Hosted API-only fixture tests were not used as browser evidence.

## Direct rendered observations

1. The seed page rendered in Safe mode without Undo; the probe was absent.
2. After prompt submission, Chromium rendered the permission controls while the
   probe remained absent.
3. Allow rendered `Edited 1 file`, `Changed files (1)`, and
   `Undo last change`; the probe digest matched the fixture contract.
4. Undo removed the probe and made Undo unavailable while
   `Changed files (1)` remained visible.
5. After the seed process stopped, the same owned Chromium process loaded a
   fresh reload process and rendered durable `Changed files (1)` without stale
   Undo; the probe remained absent.

The observation verdict is `PASS`.

## Host-local / retained artifacts

Artifacts remain outside the checkout. Operator copies used for packaging:

```text
/private/tmp/picogent-evidence-423d047/windows/
/private/tmp/picogent-evidence-423d047/aggregate/rendered-cross-platform-evidence.json
```

Artifact SHA-256:

```text
observation       e35bf94a091d07be5208e5f02f8e06e020ffa9966ec3c0635ba0c23d45cfcdd9
platform          af3bd592530c2e2849ed529d0fbeb74a46461c47a13667a7ebd643bddd2c377c
seed-manifest     e10879ee0a4d1e8bfbcacc8ca1cb7130c1357f330dcb54ff732739f106b14fbc
reload-manifest   dec96fb1e4f383f3f60c05f68472d718ce4ce1fa11223f6c6d0b395f8e4a1eff
screenshot-set    63ed70a6cc39da1fb675c05a1e2a56adc62736017c16553b2a3700cf2c123e58
initial           63d807e0efeb84d07dbf633fa96119058ef9010e1fd8d0f8a13f1d3c67fc9e00
permission        2c028a390c94a4dd321c9d46821a5fa317c14c3070a9b5019048dd7e8380ba91
allowed           09f50bf6304dd4f2150897cbe6f6f5cbf090d306171b9d0a92b5d4fdbf111517
undone            03063029ac679eb7f35e90d38fcb19bf3bc0dc20bec137c2a5f471a062747926
reload            44ec7016816cc354083b601610636f8b642cb84ca4d5aa577df49416eef68f88
cross-aggregate   d5e55136fa1af5c00b37b22d6f06d1b31752e66abe027626c23243ecc3b4702f
matrix-cross      f5610d348a0b881d72599e075dc12beedd06ebab1720c50c14e0fbdef2bfff51
matrix-full       46e3f40ca56ff3450a1047a0e48948c742b49e93f8e9b14619a6927a41d95bfe
```

The digest-only platform record contains:

```text
candidate_sha=423d0471c1864651473c2f1828b67066885f79bd
platform=windows
architecture=amd64
environment=task-owned-disposable
browser=playwright-chromium-headless-134.0.6998.35
fixture=rendered-recovery
observation_sha256=e35bf94a091d07be5208e5f02f8e06e020ffa9966ec3c0635ba0c23d45cfcdd9
screenshot_sha256=63ed70a6cc39da1fb675c05a1e2a56adc62736017c16553b2a3700cf2c123e58
verdict=PASS
source_tree_modified=false
```

## Cross-platform aggregate

Packaged from a clean detached checkout at
`423d0471c1864651473c2f1828b67066885f79bd` with the matching Darwin and Linux
platform records. Aggregate `verdict=PASS`. Exact-SHA matrix validation at that
clean checkout projected `rendered-cross-platform=PASS`.

Docs-only descendants of this tip retain live/local rendered and this aggregate
with `--behavior-sha 423d0471c1864651473c2f1828b67066885f79bd`; they do not
rebind the observation to a later non-docs tip without recollection.

## Explicit non-claims

- does not upgrade `hostile-filesystem-toctou` or `release-authorization`
- does not treat API-boundary fixture tests as owned-browser proof
- does not authorize a release by itself
