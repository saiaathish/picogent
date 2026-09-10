# v4 Darwin rendered recovery evidence at exact candidate ad34bd5

This record belongs to [#609](https://github.com/saiaathishkarthik/picogent/issues/609),
the focused Darwin refresh under
[#453](https://github.com/saiaathishkarthik/picogent/issues/453). It records one
fresh local Darwin/arm64 owned-browser observation of the real embedded GUI at
exact candidate SHA `ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c`.

The observation is bounded to Darwin rendered recovery. It does not claim
Linux or Windows behavior at this candidate, cross-platform rendered coverage,
live-provider quality, arbitrary hostile same-UID filesystem races, or release
authorization. The later exact-candidate aggregate is recorded separately in
[V4-RENDERED-CROSS-PLATFORM.md](V4-RENDERED-CROSS-PLATFORM.md).

## Provenance

- Source: clean detached clone at
  `ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c`.
- Fixture build: `vcs.revision=ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c`,
  `vcs.modified=false`, `-tags rendered_fixture`; local Go runtime was
  `go1.26.6`.
- Host/runtime: `darwin/arm64`.
- Browser: task-owned Playwright Chromium headless
  `151.0.7922.34`; fixture session `rendered-recovery-fixture`.
- Seed started at `2026-09-10T06:23:47.982831Z`; reload started at
  `2026-09-10T06:23:54.486841Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`; seed and reload fixture-process exits were
  verified.
- The disposable fixture home, workspace, browser profile, manifests, and
  screenshots remained outside the checkout under
  `/private/tmp/picogent-rendered-darwin-609/`.

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

The collection summary reported `verdict=PASS`,
`fixture_exit_verified=true`, `source_sha_verified=true`,
`source_tree_modified=false`, and 18/18 required checks true. Independently
captured SHA-256 values are:

```text
observation       d5a52ac6638a3fdc1f1f80e5e01c842d483bbe0350b92d2bf9f38dfef1b93899
platform          fc86cb56692d23cabcfbd9a26c814b20b0b08be28265e8db00556e0000c8d6b8
collection        faad616773d672cb8c8325c99182257c74b8baa9b5b1a3fd95b0c2ffaf254c1e
runtime-matrix    c2f5b63f3751f95345b901927bde00aac8b10762ffefa879ae3ec95b843a9b17
seed-manifest     d6fc4c818152e342ef4129405230c4931c7fa44ae478a4bc5793ad806b7303b4
reload-manifest   5bc23af493ee315024a17ce9334d99e1a89b4ddde50b522d5333fcece3584116
screenshot-set    fd582e69e21cd61f4e1c8a275561be21d7eb63fd523da622a668818c8781e6ea
```

The platform record declares:

```text
candidate_sha=ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c
platform=darwin
architecture=arm64
environment=task-owned-disposable
browser=playwright-chromium-headless-151.0.7922.34
fixture=rendered-recovery
verdict=PASS
source_tree_modified=false
```

The exact-head Darwin runtime matrix reported:

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
identity, UTC observation time, observation and screenshot digests, and an
explicit clean-source assertion. No DOM transcript, URL, credential, or
screenshot bytes are stored in the repository.

## Boundaries

- `CONFIRMED`: Safe permission, contained Allow, contained Undo, and durable
  fresh-process reload were directly observed on Darwin/arm64.
- `PASS`: `rendered-platform-local` for Darwin/arm64 at candidate
  `ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c`.
- `PASS` for the three-platform aggregate is recorded separately and is not
  inferred from this Darwin-only observation.
- `UNVERIFIED`: live-provider claims and broad same-UID filesystem TOCTOU.
- `INCONCLUSIVE`: release authorization; this observation does not provide an
  operator approval.

This is an evidence checkpoint, not a v4 release claim.
