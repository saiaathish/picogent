# v4 Windows rendered recovery evidence at tip 18dc4a1

This record adds the direct Windows owned-browser observation required by
[#507](https://github.com/saiaathish/picogent/issues/507), under parent
[#453](https://github.com/saiaathish/picogent/issues/453), at exact behavior
SHA `18dc4a1ca1137ab78dfd0102848eb99c417653cc`.

It proves the `windows/amd64` rendered recovery path in a task-owned disposable
Playwright Chromium profile on GitHub Actions `windows-latest`. Combined with
the matching Darwin and Linux observations at the same SHA, the three-input
packager produced a PASS aggregate. This record does not upgrade
hostile-filesystem TOCTOU or release authorization.

## Provenance

- Source: clean detached checkout at
  `18dc4a1ca1137ab78dfd0102848eb99c417653cc` inside the hosted Windows runner.
- Fixture binary:
  `vcs.revision=18dc4a1ca1137ab78dfd0102848eb99c417653cc`,
  `vcs.modified=false` (verified via `go version -m` before the browser run).
- Runtime: GitHub Actions `windows-latest` (Windows Server 2025), Go `1.25.x`.
- Owned browser: Playwright Chromium `134.0.6998.35`, headless, disposable
  persistent profile under Local Temp.
- Fixture: `go-build-tags-rendered_fixture`; session
  `rendered-recovery-fixture`.
- Seed started at `2026-09-07T03:26:17.0519249Z`; reload started in a fresh
  fixture process at `2026-09-07T03:26:22.2320882Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`.
- Probe content SHA-256:
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.
- Collection run:
  [34079577288](https://github.com/saiaathish/picogent/actions/runs/34079577288)
  (`pull_request` on collector PR #524); push twin
  [34079574280](https://github.com/saiaathish/picogent/actions/runs/34079574280).

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
/tmp/picogent-cross-18dc4a1/windows-rendered-platform.json
/tmp/picogent-cross-18dc4a1/rendered-cross-platform-evidence.json
```

Artifact SHA-256:

```text
observation       3dae1f16808927ab2cddf5ebb4bae51d36f32ef12687c2e9e7c0755889904537
platform          45d49056b51140ab8798ce3051d3480684ab49644470f2879ee2715748c9de4a
seed-manifest     cb78d10decb83f7e4221fbac0e4c2a6b10b32430cc861a4b5fc81ce1f3ce7429
reload-manifest   618f20377b1fafc905aefaf72d49e671eea5ebf290389142d5f413c815d2494c
screenshot-set    34ef70b68639dffcd9462bd3c0beb7dfbbbe68e11f31fa224cb397c0b0c6f176
initial           bc36699a657eb8d546731427fb551cbda84a10cffaa8722637a038324cb37a4c
permission        2513064e6cffd82ba2d8b639806e5fc4c66a6b3087e0547651eba89765f54d2c
allowed           893923b42116e6cd2c6858c93640e51225dca87a840aa97b135ec0a47083c27c
undone            a7331674e17525492bc9a4f3dfd44f2bb6ee0a28b74b464bba34253b924c85ca
reload            626e75fbba8bff51015450c80fffe652c7726dd9f3bb6e5eb2969ef77bdfee3f
cross-aggregate   e519f22aa878c33479ee7e85282a8e00e4767d68560a9ff537601c1803a23ef5
matrix-at-18dc4a1 0b62129e0cae2aa0c30d70b625d630ba20e1448f6b8f4d23fcf7f155c920523e
```

The digest-only platform record contains:

```text
candidate_sha=18dc4a1ca1137ab78dfd0102848eb99c417653cc
platform=windows
architecture=amd64
environment=task-owned-disposable
browser=playwright-chromium-headless-134.0.6998.35
fixture=rendered-recovery
observation_sha256=3dae1f16808927ab2cddf5ebb4bae51d36f32ef12687c2e9e7c0755889904537
screenshot_sha256=34ef70b68639dffcd9462bd3c0beb7dfbbbe68e11f31fa224cb397c0b0c6f176
verdict=PASS
source_tree_modified=false
```

## Cross-platform aggregate

Packaged from a clean detached checkout at
`18dc4a1ca1137ab78dfd0102848eb99c417653cc` with the Darwin, Linux, and Windows
platform artifacts above:

```text
schema=picogent.v4.rendered-cross-platform-evidence.v1
candidate_sha=18dc4a1ca1137ab78dfd0102848eb99c417653cc
verdict=PASS
required_platforms=darwin,linux,windows
aggregate_sha256=e519f22aa878c33479ee7e85282a8e00e4767d68560a9ff537601c1803a23ef5
```

Exact-SHA matrix validation at that checkout with
`PICOGENT_RENDERED_CROSS_PLATFORM_EVIDENCE=1` projected:

```text
HEAD=PASS  tree=CLEAN  behavior_provenance=EXACT_HEAD
PASS: 9  INCONCLUSIVE: 1  UNVERIFIED: 1
rendered-cross-platform=PASS
rendered-platform-local=PASS
live-provider-connectivity=PASS
live-provider-quality=PASS
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

A later docs-only tip may document this aggregate, but
`rendered-cross-platform` remains exact-candidate bound: validate it from a
clean checkout at `18dc4a1…`, not by rebinding a tip SHA through
`--behavior-sha`. Hostile TOCTOU and release authorization stay unchanged.
