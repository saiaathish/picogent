# v4 Darwin rendered recovery evidence at current candidate c8a9eda

This record belongs to [#605](https://github.com/saiaathishkarthik/picogent/issues/605),
the focused Darwin child under
[#453](https://github.com/saiaathishkarthik/picogent/issues/453). It records one
fresh local Darwin/arm64 owned-browser observation of the real embedded GUI at
exact candidate SHA `c8a9eda1c1965c29d10046a3103e1442766e6e94`.

The observation is bounded to Darwin rendered recovery. It does not claim
Linux or Windows behavior at this candidate, cross-platform rendered coverage,
live-provider quality, arbitrary hostile same-UID filesystem races, or release
authorization.

## Provenance

- Source: clean detached clone at
  `c8a9eda1c1965c29d10046a3103e1442766e6e94`.
- Fixture build: `vcs.revision=c8a9eda1c1965c29d10046a3103e1442766e6e94`,
  `vcs.modified=false`, `-tags rendered_fixture`; local Go runtime was
  `go1.26.6`.
- Host/runtime: `darwin/arm64`.
- Browser: task-owned Playwright Chromium headless
  `151.0.7922.34`; fixture session `rendered-recovery-fixture`.
- Seed started at `2026-09-10T06:03:36.088785Z`; reload started in a fresh
  fixture process at `2026-09-10T06:03:42.528835Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`; the collection verified fixture-process exit.
- The disposable fixture home, workspace, browser profile, manifests, and
  screenshots remained outside the checkout under
  `/private/tmp/picogent-rendered-darwin-605/`.

## Direct rendered observations

| Sequence | Direct UI/DOM observation | Local corroboration | Verdict |
| --- | --- | --- | --- |
| 1 | Fresh seed page loaded in Safe mode with no enabled Undo control. | Seed manifest recorded the probe as absent before the turn. | `PASS` |
| 2 | Submitting the probe request rendered the Safe permission controls while the probe remained absent. | Seed manifest recorded exact clean provenance and no probe before Allow. | `PASS` |
| 3 | Clicking `Allow` rendered `Edited 1 file`, `Changed files (1)`, and enabled `Undo last change`. | Observation recorded the one-file mutation, changed-file count, and enabled Undo. | `PASS` |
| 4 | Clicking `Undo last change` removed the Undo control. | Probe was absent after Undo and the Undo control was disabled. | `PASS` |
| 5 | After the seed process stopped, the owned browser loaded a fresh reload process and rendered durable `Changed files (1)` without stale Undo. | Reload manifest retained exact clean provenance; probe was absent after reload. | `PASS` |

The `Changed files (1)` value after Undo and reload is retained task history; it
does not mean the undone path still exists.

## Retained digest-only artifacts

The collection summary reported `verdict=PASS`, `fixture_exit_verified=true`,
and 18/18 required checks true. Independently captured SHA-256 values are:

```text
observation       d1fabea02577f907e162daf44f5f9d2669efd4a901b58bc0d633d65a5db39a6b
platform          e892ea7b2e89b1760c7fb2f47a4f690763c35951f450ea2e365011ae69f0a02e
collection        b8ef73e5701088591ee7c0785d7b2883a6942947ecc7b2812b0df359f9f7842c
seed-manifest     921cea64e93b7c3922339849e9190411bc9b233d53ebed23bc4174bb9253ef4b
reload-manifest   7596b8321d7b15896d1f2ca530d7a45ca0d68e49e35bc9e1e32070e1021b1619
runtime-matrix    5d7040d4a027977339b2e4b4de3f5b743078ea55e353d5ee8fe09b978d1f9f63
screenshot-set    658b4590f8e79474acb73eb9cf68a1d88887ed44f20bca9137d5f59b9801adc2
```

The platform record declares:

```text
candidate_sha=c8a9eda1c1965c29d10046a3103e1442766e6e94
platform=darwin
architecture=arm64
environment=task-owned-disposable
browser=playwright-chromium-headless-151.0.7922.34
fixture=rendered-recovery
verdict=PASS
source_tree_modified=false
```

The exact-head runtime matrix from the clean detached clone reported:

```text
HEAD=PASS  tree=CLEAN  host=darwin/arm64
PASS: 7  INCONCLUSIVE: 1  UNVERIFIED: 4
rendered-platform-local=PASS
rendered-cross-platform=UNVERIFIED
live-provider-connectivity=UNVERIFIED
live-provider-quality=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

The Darwin platform record uses schema
`picogent.v4.rendered-platform-evidence.v1`, an exact candidate SHA,
`task-owned-disposable`, the declared Darwin architecture, the browser/fixture
identity, observation and screenshot digests, and an explicit clean-source
assertion. No DOM transcript, URL, credential, or screenshot bytes are stored
in the repository.

## Boundaries

- `CONFIRMED`: Safe permission, contained Allow, contained Undo, and durable
  fresh-process reload were directly observed on Darwin/arm64.
- `PASS`: `rendered-platform-local` for Darwin/arm64 at candidate
  `c8a9eda1c1965c29d10046a3103e1442766e6e94`.
- `UNVERIFIED`: Linux, Windows, and the exact-candidate cross-platform rendered
  claim.
- `UNVERIFIED`: live-provider claims and broad same-UID filesystem TOCTOU.
- `INCONCLUSIVE`: release authorization; this observation does not provide an
  operator approval.

This is an evidence checkpoint, not a v4 release claim.
