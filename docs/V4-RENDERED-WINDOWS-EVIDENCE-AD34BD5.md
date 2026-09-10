# v4 Windows rendered recovery evidence at exact candidate ad34bd5

This record belongs to [#608](https://github.com/saiaathishkarthik/picogent/issues/608),
the focused Windows refresh under
[#453](https://github.com/saiaathishkarthik/picogent/issues/453). It records one
fresh hosted Windows/amd64 owned-browser observation of the real embedded GUI
at exact candidate SHA `ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c`.

The observation is bounded to Windows rendered recovery. It does not by itself
claim cross-platform rendered coverage, live-provider quality, arbitrary
hostile same-UID filesystem races, or release authorization. The exact-
candidate three-platform aggregate is recorded separately in
[V4-RENDERED-CROSS-PLATFORM.md](V4-RENDERED-CROSS-PLATFORM.md).

## Provenance

- Hosted run: [GitHub Actions run 34444672008](https://github.com/saiaathishkarthik/picogent/actions/runs/34444672008).
- Source: clean exact-SHA checkout of
  `ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c` on the hosted Windows runner.
- Fixture: `go-build-tags-rendered_fixture`; the workflow built it from the
  exact checked-out candidate and verified the clean source checkout before
  collection.
- Runtime: `windows/amd64`, Go `go1.25.14`.
- Browser: task-owned Chrome headless `152.0.7977.83`; fixture session
  `rendered-recovery-fixture`.
- Seed started at `2026-09-10T06:20:12.1675002Z`; reload started at
  `2026-09-10T06:20:21.1653278Z`; observation completed at
  `2026-09-10T06:20:21.389Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`; both seed and reload fixture processes exited
  before the observation was published.
- The disposable fixture home, workspace, browser profile, manifests, and
  screenshot remained outside the checkout in the hosted runner temp area.

## Direct rendered observations

The Windows collector drove the normal embedded GUI through the complete
allow/undo/reload sequence. Its observation record set all behavioral
assertions below to true, with `source_tree_modified=false` as the explicit
clean-source assertion:

| Sequence | Direct UI/DOM observation | Corroboration | Verdict |
| --- | --- | --- | --- |
| 1 | Fresh seed page rendered with the Undo control disabled. | `seed_initial_undo_disabled=true`; seed manifest recorded the probe as absent. | `PASS` |
| 2 | The Safe permission prompt rendered for the contained recovery probe. | `permission_rendered=true`; the collector checked the probe-specific permission text. | `PASS` |
| 3 | Clicking `Allow` rendered the contained one-file mutation, including `Edited 1 file` and `Changed files (1)`. | `allow_rendered_contained_mutation=true`; the probe content matched the fixture contract. | `PASS` |
| 4 | Clicking Undo rendered restoration and removed the probe. | `undo_rendered_restoration=true`; the probe was absent before reload. | `PASS` |
| 5 | A fresh reload process rendered durable `Changed files (1)` with Undo disabled. | `reload_rendered_durable_history=true`, `reload_undo_disabled=true`, and `probe_absent_after_reload=true`. | `PASS` |

## Retained digest-only artifacts

The platform and observation records agree on the exact candidate, platform,
architecture, browser, and fixture. Both fixture exits were verified.
Independently captured SHA-256 values are:

```text
observation       847a0f53a0c6a34b40cdb84ca6160499d73b888011de7ff696db5921af5e9d53
platform          923804ee1c9ec44ae48cd6ce023ec10a34ed07e240e6b4a9f8dce73a54f77485
seed-manifest     2c8a030223c1c71a2bd12cf44a1f52747f16ec118824a9da7aa75b7b8b57b2d4
reload-manifest   c7d08eeae73c75b070765550098f53eae770ad5b70b09af1e234b2a242731d46
runtime-matrix    9df7676d6e87f09d4e3148e597c3f87d494a0c9401a34ab464f2c7e6c21a6ed0
screenshot        0836d20e73d94e1ab2f91e492c6485cbcd3e2b694179fc789de88d19fdd99c7d
```

The platform record declares:

```text
candidate_sha=ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c
platform=windows
architecture=amd64
environment=task-owned-disposable
browser=chrome-headless-152.0.7977.83
fixture=rendered-recovery
verdict=PASS
source_tree_modified=false
```

The hosted exact-head runtime matrix reported:

```text
HEAD=PASS  tree=CLEAN  host=windows/amd64
PASS: 7  INCONCLUSIVE: 1  UNVERIFIED: 4
rendered-platform-local=PASS
rendered-cross-platform=UNVERIFIED
live-provider-connectivity=UNVERIFIED
live-provider-quality=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

The Windows platform record uses schema
`picogent.v4.rendered-platform-evidence.v1`, an exact candidate SHA,
`task-owned-disposable`, the declared Windows architecture, the browser/fixture
identity, UTC observation time, observation and screenshot digests, and an
explicit clean-source assertion. No DOM transcript, URL, credential, or
screenshot bytes are stored in the repository.

## Boundaries

- `CONFIRMED`: Safe permission, contained Allow, contained Undo, and durable
  fresh-process reload were directly observed on Windows/amd64.
- `PASS`: `rendered-platform-local` for Windows/amd64 at candidate
  `ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c`.
- `PASS` for the three-platform aggregate is recorded separately and is not
  inferred from this Windows-only observation.
- `UNVERIFIED`: live-provider claims and broad same-UID filesystem TOCTOU.
- `INCONCLUSIVE`: release authorization; this observation does not provide an
  operator approval.

This is an evidence checkpoint, not a v4 release claim.
