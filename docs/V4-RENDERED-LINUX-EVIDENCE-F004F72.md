# v4 Linux rendered recovery evidence at current candidate f004f72

This record belongs to [#601](https://github.com/saiaathishkarthik/picogent/issues/601),
the focused Linux child under
[#453](https://github.com/saiaathishkarthik/picogent/issues/453). It records one
fresh hosted Linux owned-browser observation of the real embedded GUI at exact
candidate SHA `f004f724a4336f948e765d91c6a985b01eb37505`.

The observation is bounded to Linux rendered recovery. It does not claim
Darwin or Windows behavior at this candidate, cross-platform rendered
coverage, live-provider quality, arbitrary hostile same-UID filesystem races,
or release authorization.

## Provenance

- Hosted run: [GitHub Actions run 34440445794](https://github.com/saiaathishkarthik/picogent/actions/runs/34440445794).
- Source: clean exact-SHA checkout of
  `f004f724a4336f948e765d91c6a985b01eb37505` on the hosted Linux runner.
- Fixture build: `vcs.revision=f004f724a4336f948e765d91c6a985b01eb37505`,
  `vcs.modified=false`, `-tags rendered_fixture`; build-info SHA-256 is
  `45f6a195c8dcf75af7e5fb5add3d78abe2c6b598257083117564aad25d117afa`.
- Runtime: `linux/amd64`, Go `go1.25.14`.
- Browser: task-owned Playwright Chromium headless
  `134.0.6998.35`; fixture session `rendered-recovery-fixture`.
- Seed started at `2026-09-10T05:17:50.261558837Z`; reload started in a fresh
  fixture process at `2026-09-10T05:17:57.270325093Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`; the collection verified fixture-process exit.
- The disposable fixture home, workspace, browser profile, manifests, and
  screenshots remained outside the checkout under
  `/private/tmp/picogent-rendered-linux-601/`.

## Direct rendered observations

| Sequence | Direct UI/DOM observation | Local corroboration | Verdict |
| --- | --- | --- | --- |
| 1 | Fresh seed page loaded in Safe mode with no enabled Undo control. | Seed manifest recorded the probe as absent before the turn. | `PASS` |
| 2 | Submitting the probe request rendered the Safe permission controls while the probe remained absent. | Seed manifest recorded exact clean provenance and no probe before Allow. | `PASS` |
| 3 | Clicking `Allow` rendered `Edited 1 file`, `Changed files (1)`, and enabled `Undo last change`. | Observation recorded `edited_one_file_visible=true`, `changed_files_one_visible_after_allow=true`, and `undo_visible_after_allow=true`. | `PASS` |
| 4 | Clicking `Undo last change` removed the Undo control. | Probe was absent after Undo and `undo_visible_after_undo=false`. | `PASS` |
| 5 | After the seed process stopped, the owned browser loaded a fresh reload process and rendered durable `Changed files (1)` without stale Undo. | Reload manifest retained exact clean provenance; probe was absent after reload. | `PASS` |

The `Changed files (1)` value after Undo and reload is retained task history; it
does not mean the undone path still exists.

## Retained digest-only artifacts

The collection summary reported `verdict=PASS`, `fixture_exit_verified=true`,
and 18/18 required checks true. Independently captured SHA-256 values are:

```text
observation       6681324a7e417312e47fa94bb53719929dad3a0974645cfbfda861af99a939ef
platform          276ced42c51fcde994b9890da6e3cf94a0937231707c7793fd13a787ebe59a0f
collection        cfc8645482d92cba88e97d71b6e7e1096fb606995944bef5229040b23e33aa1f
seed-manifest     bdc0f3275a66a88b7e4f935ce63fa191e2b93a09bdaa462a2c8128c1b569b84d
reload-manifest   884c899e1fb15aa258cb69ded5ed1a51f10ca012ac9d519d494be2a0e7dd5fc9
runtime-matrix    dee13285d9b702ccbbd0fb3f35b2e4a58f76995e35a204cb983848b0a1c7cd74
screenshot-set    17d653eb5274faf735d1e78a1cd80d7be5b0117311e6f62ec88e4c247e1fb216
```

The platform record declares:

```text
candidate_sha=f004f724a4336f948e765d91c6a985b01eb37505
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
identity, observation and screenshot digests, and an explicit clean-source
assertion. No DOM transcript, URL, credential, or screenshot bytes are stored
in the repository.

## Boundaries

- `CONFIRMED`: Safe permission, contained Allow, contained Undo, and durable
  fresh-process reload were directly observed on Linux/amd64.
- `PASS`: `rendered-platform-local` for Linux/amd64 at candidate
  `f004f724a4336f948e765d91c6a985b01eb37505`.
- `UNVERIFIED`: Windows and the exact-candidate cross-platform rendered claim.
- `UNVERIFIED`: live-provider claims and broad same-UID filesystem TOCTOU.
- `INCONCLUSIVE`: release authorization; this observation does not provide an
  operator approval.

This is an evidence checkpoint, not a v4 release claim.
