# v4 Linux rendered recovery evidence at exact candidate ad34bd5

This record belongs to [#607](https://github.com/saiaathishkarthik/picogent/issues/607),
the focused Linux refresh under
[#453](https://github.com/saiaathishkarthik/picogent/issues/453). It records one
fresh hosted Linux/amd64 owned-browser observation of the real embedded GUI at
exact candidate SHA `ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c`.

The observation is bounded to Linux rendered recovery. It does not claim
Darwin or Windows behavior at this candidate, cross-platform rendered
coverage, live-provider quality, arbitrary hostile same-UID filesystem races,
or release authorization.

## Provenance

- Hosted run: [GitHub Actions run 34444670203](https://github.com/saiaathishkarthik/picogent/actions/runs/34444670203).
- Source: clean exact-SHA checkout of
  `ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c` on the hosted Linux runner.
- Fixture build: `vcs.revision=ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c`,
  `vcs.modified=false`, `-tags rendered_fixture`; local Go runtime was
  `go1.25.14`.
- Runtime: `linux/amd64`.
- Browser: task-owned Playwright Chromium headless
  `134.0.6998.35`; fixture session `rendered-recovery-fixture`.
- Seed started at `2026-09-10T06:20:09.650747831Z`; reload started at
  `2026-09-10T06:20:15.996964442Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`; the collection verified fixture-process exit.
- The disposable fixture home, workspace, browser profile, manifests, and
  screenshots remained outside the checkout under
  `/private/tmp/picogent-rendered-linux-607/`.

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
observation       03accda99a641d3911fa1fa0755817e7877bf2c984b647b6883f3951e35c67f0
platform          7e58d5745bf179e78ab38500fa1c706179a763c628a2dd67e7d57eb867e3e069
collection        9e073f7002361d659b8e0b4455e7c295a449a93ebfc8229d8ecafa2f96222ec4
runtime-matrix    c0e9fbcdfbd57c3606bf5076eecd4adc80b697e43f9ab084763ba3545efbb2cc
seed-manifest     a6c97a0d128578260316639a3351e3518c1773661f73fe3f279e311d0ae2c508
reload-manifest   6b83dd407ab81ffa9c7de89381ae28b310d544e5d196c6c13706bfda9ea3bd6f
screenshot-set    b9e8da2e695234bd1d9e1eae0a79f0d1f45d42af9bf28962a91a80e15bcb7644
```

The platform record declares:

```text
candidate_sha=ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c
platform=linux
architecture=amd64
environment=task-owned-disposable
browser=playwright-chromium-headless-134.0.6998.35
fixture=rendered-recovery
verdict=PASS
source_tree_modified=false
```

The hosted exact-head runtime matrix reported:

```text
HEAD=PASS  tree=CLEAN  host=linux/amd64
PASS: 7  INCONCLUSIVE: 1  UNVERIFIED: 4
rendered-platform-local=PASS
rendered-cross-platform=UNVERIFIED
live-provider-connectivity=UNVERIFIED
live-provider-quality=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

The Linux platform record uses schema
`picogent.v4.rendered-platform-evidence.v1`, an exact candidate SHA,
`task-owned-disposable`, the declared Linux architecture, the browser/fixture
identity, UTC observation time, observation and screenshot digests, and an
explicit clean-source assertion. No DOM transcript, URL, credential, or
screenshot bytes are stored in the repository.

## Boundaries

- `CONFIRMED`: Safe permission, contained Allow, contained Undo, and durable
  fresh-process reload were directly observed on Linux/amd64.
- `PASS`: `rendered-platform-local` for Linux/amd64 at candidate
  `ad34bd534b7f9aaf9a3ccc508940dc8ff171bf8c`.
- `PASS` for the three-platform aggregate is recorded separately and is not
  inferred from this Linux-only observation.
- `UNVERIFIED`: live-provider claims and broad same-UID filesystem TOCTOU.
- `INCONCLUSIVE`: release authorization; this observation does not provide an
  operator approval.

This is an evidence checkpoint, not a v4 release claim.
