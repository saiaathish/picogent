# v4 rendered recovery evidence at d1298998

This record belongs to [#507](https://github.com/saiaathish/picogent/issues/507)
under parent [#453](https://github.com/saiaathish/picogent/issues/453). It
records a fresh macOS-only BrowserOS Neo observation of the rendered
allow→undo→fresh-process reload flow against merged `main`
`d1298998cc000c40ad2fdaf71a24e17649e09e19`.

This is a direct rendered observation, not cross-platform evidence. It does not
close #507, upgrade the Linux or Windows rows, or authorize a release.

## Provenance

- Source SHA: `d1298998cc000c40ad2fdaf71a24e17649e09e19`.
- Checkout: clean detached verification worktree at the observation SHA;
  `git diff --check` passed.
- Fixture manifest: `source_tree_modified=false`, but
  `source_sha_verified=false`. The supplied SHA and clean checkout were
  recorded; the build did not expose a matching compiled VCS revision, so this
  record does not promote a matrix `PASS`.
- Runtime: `go-build-tags-rendered_fixture`.
- Host: `darwin/arm64`, Darwin `25.5.0`, Go `1.26.6`.
- Browser: task-owned BrowserOS Neo session `codex/rendered-evidence`, page
  `391`.
- Fixture session: `rendered-recovery-fixture`.
- Seed URL: `http://127.0.0.1:58856/`.
- Reload URL: `http://127.0.0.1:59900/`.
- Seed started at `2026-09-07T01:08:21.883781Z`; reload started at
  `2026-09-07T01:22:24.458867Z`.
- Probe content SHA-256:
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.

The disposable fixture home, workspace, and manifests remain outside the
checkout. Screenshots were not retained and are `UNRECORDED`.

## Direct rendered observations

| Sequence | Direct browser observation | Local corroboration | Verdict |
| --- | --- | --- | --- |
| 1 | The fresh seed page opened in Safe mode with no Undo control. | The seed manifest recorded the probe as absent before the turn. | `CONFIRMED` |
| 2 | Submitting `Create the rendered recovery probe file` displayed the Safe-mode `Deny`, `This turn`, `Always allow`, and `Allow` controls while the probe remained absent. | The fixture used the contained disposable workspace and deterministic provider. | `CONFIRMED` |
| 3 | Clicking `Allow` rendered `Edited 1 file`, `Changed files (1)`, and an enabled `Undo last change` control. | `rendered-recovery-probe.txt` contained `rendered recovery fixture\n` with the contract hash above. | `CONFIRMED` |
| 4 | Clicking `Undo last change` removed the Undo control. | The probe path and the fixture undo record were both absent afterward. | `CONFIRMED` |
| 5 | Navigating the same task-owned tab to the reload URL showed the durable task history and `Completion proof pending`; no stale completion proof or `Undo last change` control was rendered. | The probe remained absent after the fresh process loaded the task. | `CONFIRMED` |

The browser text and accessibility tree are the direct UI evidence for this
run. No live-provider quality, arbitrary-repository, hostile-writer,
unsupported-platform, or release-readiness claim is inferred from it.

## Bounded status

| Claim | Status |
| --- | --- |
| macOS rendered allow→undo→fresh-process reload behavior | `CONFIRMED` |
| Exact-candidate rendered-platform matrix `PASS` | `UNVERIFIED` because `source_sha_verified=false` and no digest-only artifact was retained |
| Linux rendered behavior | `UNVERIFIED` |
| Windows rendered behavior | `UNVERIFIED` |
| Cross-platform rendered aggregate | `UNVERIFIED` |
| Release authorization | unchanged; not claimed |

The Linux and Windows owned-browser artifacts required by #507 are still
outstanding. If a later candidate SHA is used for that issue, recollect all
three platform observations at the same candidate rather than rebinding this
macOS-only record.
