# v4 rendered recovery evidence at current main

This record belongs to [#492](https://github.com/saiaathish/picogent/issues/492),
the small direct-observation child under
[#453](https://github.com/saiaathish/picogent/issues/453). It refreshes the
rendered recovery observation against the exact merged `main` tip. The record
is intentionally bounded: it does not claim live-provider quality,
cross-platform behavior, hostile-writer safety, or release readiness.

The temporary home, manifests, and matrix artifact named below are
author-host-local paths from the observation and are not durable PR artifacts.
The digest-only evidence record and matrix summary are preserved in the
[#492 issue record](https://github.com/saiaathish/picogent/issues/492).

## Provenance

- Source SHA: `a4c825b4f451506c0072a64183bca724f33d4b55`.
- Fixture binary identity: `vcs.revision=a4c825b4f451506c0072a64183bca724f33d4b55`,
  `vcs.modified=false`.
- Runtime: `go-build-tags-rendered_fixture`.
- Host: `darwin/arm64`.
- Browser session: `codex/rendered-evidence`.
- Browser tab: task-owned BrowserOS neo page `339`.
- Seed URL: `http://127.0.0.1:60425/`.
- Reload URL: `http://127.0.0.1:60572/`.
- Fixture session: `rendered-recovery-fixture`.
- Author-host-local disposable fixture home (not retained in this PR):
  `/var/folders/z_/zxn_ghn96dd_78qxc_dfh9_00000gq/T/picogent-rendered-recovery-home-3601527703`.
- Author-host-local workspace (not retained in this PR):
  `/var/folders/z_/zxn_ghn96dd_78qxc_dfh9_00000gq/T/picogent-rendered-recovery-home-3601527703/workspace`.
- Author-host-local seed manifest (not retained in this PR):
  `/var/folders/z_/zxn_ghn96dd_78qxc_dfh9_00000gq/T/picogent-rendered-recovery-home-3601527703/rendered-recovery-fixture-seed.json`.
- Author-host-local reload manifest (not retained in this PR):
  `/var/folders/z_/zxn_ghn96dd_78qxc_dfh9_00000gq/T/picogent-rendered-recovery-home-3601527703/rendered-recovery-fixture-reload.json`.
- The fixture manifests recorded `issue=467`, `parent_issue=453`;
  `467` identifies the fixture implementation lane, while this fresh
  current-main evidence checkpoint belongs to `#492`.
- Both manifests recorded
  `source_sha_verified=true`, and `source_tree_modified=false`.
- Both manifests recorded probe SHA-256
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.
- Seed started at `2026-09-06T10:47:35.078035Z`; reload started at
  `2026-09-06T10:49:38.369536Z`.
- Matrix generated at `2026-09-06T10:52:21Z`; the author-host-local rendered
  evidence artifact used for that run was
  `/private/tmp/picogent-rendered-platform-a4c825b.json` and is not retained
  in this PR. Its digest-only contents are preserved in the #492 issue record.
- Browser action timestamps and durable screenshot paths were not exposed;
  both are therefore `UNRECORDED`. Screenshots were inspected inline at the
  permission, post-Allow, post-Undo, and reload checkpoints.

The fixture uses the real embedded GUI, Safe permission flow, `/api/permission`,
`/api/chat`, SSE updates, task/session persistence, and checkpoint-backed Undo.
Its provider is deterministic and local by design.

## Direct rendered observations

| Sequence | Boundary and direct UI observation | Local corroboration | Verdict |
| --- | --- | --- | --- |
| 1 | Fresh seed page loaded in Safe mode with no Undo control. | Seed manifest recorded the probe as initially absent. | `PASS` |
| 2 | The submitted task rendered the Safe permission card with `Deny`, `This turn`, `Always allow`, and `Allow`; the probe was not yet present. | The fixture started from a fresh contained workspace. | `PASS` |
| 3 | Clicking `Allow` rendered `Edited 1 file`, `Changed files (1)`, and an enabled `Undo last change` control. | The probe existed at 26 bytes with the expected SHA-256. | `PASS` |
| 4 | Clicking `Undo last change` removed the recovery banner/control and rendered `Undid last turn: removed rendered-recovery-probe.txt`. | The probe path was absent. | `PASS` |
| 5 | After stopping the seed process and loading the reload URL in the same task-owned tab, the prior task/history rendered with `Changed files (1)` but no stale recovery banner or Undo control. | The probe remained absent after fresh-process state load. | `PASS` |

The `Changed files (1)` value in sequences 4 and 5 is retained task history; it
is not evidence that the undone path still exists.

## Matrix result

The exact-head matrix was run from a clean checkout at the exact source SHA
with the digest-only rendered artifact and recorded:

```text
HEAD=PASS  tree=CLEAN
PASS: 6  INCONCLUSIVE: 1  UNVERIFIED: 4
rendered-platform-local=PASS
rendered-cross-platform=UNVERIFIED
live-provider-quality=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

The local run used observation digest
`9cc265674aa47e0dab903562244e64bc3bb7dfbe3218e54a3d6d12a9a80b254b` and
`screenshot_sha256: UNRECORDED`. The matrix loader retained the artifact digest
`926656ae9e4475334e6301412a7f9f188509f742d5d3112965f90b509a3378ff`.

## Acceptance boundaries

- `CONFIRMED`: Safe permission renders before the contained mutation.
- `CONFIRMED`: Allow performs the expected contained mutation.
- `CONFIRMED`: Undo restores the workspace and removes user-visible Undo.
- `CONFIRMED`: fresh-process reload preserves durable history without stale
  recovery state.
- `CONFIRMED`: source provenance is exact and clean at build time.
- `PASS`: local rendered-platform matrix row for `darwin/arm64`.
- `UNVERIFIED`: rendered behavior on Windows and Linux, or any cross-platform
  claim.
- `UNVERIFIED`: live-provider quality, arbitrary hostile same-UID filesystem
  races, and durable screenshot retention.
- `INCONCLUSIVE`: overall release authorization.

## Reproduction outline

1. Start from a clean checkout at the exact source SHA and build the fixture
   with `-tags rendered_fixture`; verify `go version -m` reports the matching
   `vcs.revision` and `vcs.modified=false`.
2. Follow [the rendered recovery runbook](V4-RENDERED-RECOVERY-FIXTURE.md) in
   a task-owned BrowserOS neo tab.
3. Observe the permission, Allow, Undo, and fresh-process reload states; keep
   the browser and filesystem observations separate from the digest-only
   matrix artifact.
4. Run the matrix against the same clean source SHA with the artifact retained
   outside the checkout.

This record is a current-main rendered checkpoint, not a v4 release claim.
