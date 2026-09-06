# v4 rendered recovery direct browser evidence

This record belongs to [#467](https://github.com/saiaathishkarthik/picogent/issues/467),
the large rendered-recovery lane under [#453](https://github.com/saiaathishkarthik/picogent/issues/453).
It records one complete, task-owned BrowserOS neo observation of the local
deterministic recovery fixture. It is bounded evidence; it is not a
live-provider, cross-platform-rendered, hostile-writer, or release-readiness
claim.

## Provenance

- Fixture source/build commit: `a3e831d00d590baa0fdb51f7aeec5e1fbc67aadd`.
- Fixture binary identity: `vcs.revision=a3e831d00d590baa0fdb51f7aeec5e1fbc67aadd`,
  `vcs.modified=false`.
- Runtime: `go-build-tags-rendered_fixture`.
- Fixture session: `rendered-recovery-fixture`.
- Browser session: `codex/recovery-evidence`.
- Seed tab: task-owned BrowserOS page `336`, URL `http://127.0.0.1:61098/`.
- Reload tab: fresh task-owned BrowserOS page `337`, URL
  `http://127.0.0.1:61662/`.
- Seed manifest:
  `/var/folders/z_/zxn_ghn96dd_78qxc_dfh9_00000gq/T/picogent-rendered-recovery-home-3014028752/rendered-recovery-fixture-seed.json`.
- Reload manifest:
  `/var/folders/z_/zxn_ghn96dd_78qxc_dfh9_00000gq/T/picogent-rendered-recovery-home-3014028752/rendered-recovery-fixture-reload.json`.
- Disposable fixture home:
  `/var/folders/z_/zxn_ghn96dd_78qxc_dfh9_00000gq/T/picogent-rendered-recovery-home-3014028752`.
- Workspace:
  `/var/folders/z_/zxn_ghn96dd_78qxc_dfh9_00000gq/T/picogent-rendered-recovery-home-3014028752/workspace`.
- Probe: `rendered-recovery-probe.txt`.
- Both manifests recorded `issue=467`, `parent_issue=453`,
  `source_sha_verified=true`, and `source_tree_modified=false`.
- Both manifests recorded the same expected probe content hash:
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.
- Seed process started at `2026-09-06T05:14:22.649418Z`; reload process started
  at `2026-09-06T05:19:53.905608Z`.
- Final observation was recorded at `2026-09-06T05:21:02.679046Z`.
- Browser action timestamps were not exposed individually and are therefore
  `UNRECORDED`; the table below preserves the observed live order.
- Screenshots were inspected inline at the permission, post-undo, and reload
  checkpoints. BrowserOS did not return a durable filesystem path, so persisted
  screenshot status is `UNRECORDED`.

The provider was the fixture's deterministic local scripted provider. No live
provider or external account behavior was exercised.

## Direct browser observations

Each row below is a direct DOM/UI observation from a task-owned tab. The
filesystem checks are separate local corroboration from the same disposable
workspace.

| Sequence | Setup and boundary | Directly observed rendered artifact | Filesystem corroboration | Verdict | Observed timestamp | Exact source SHA |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Seed phase, page `336`, Safe mode, before the prompt | The fresh page loaded with no undo control. | Seed manifest recorded the probe's expected initial state as `absent`. | `PASS` | `UNRECORDED` (action timestamp not exposed) | `a3e831d00d590baa0fdb51f7aeec5e1fbc67aadd` |
| 2 | Seed phase, page `336`, Safe-mode permission gate | The DOM showed the fixture prompt, `Changed files (0)`, and `Deny`, `This turn`, `Always allow`, and `Allow` controls. The permission card identified `write rendered-recovery-probe.txt`. | No pre-allow filesystem command was durably recorded; the seed manifest was created from a fresh contained workspace with `before=absent`. | `PASS` | `UNRECORDED` (action timestamp not exposed) | `a3e831d00d590baa0fdb51f7aeec5e1fbc67aadd` |
| 3 | Seed phase, page `336`, after clicking `Allow` | The DOM showed `Edited 1 file`, `Changed files (1)`, and an enabled `Undo last change` control. | `rendered-recovery-probe.txt` existed with 26 bytes and the expected SHA-256 `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`. | `PASS` | `UNRECORDED` (action timestamp not exposed) | `a3e831d00d590baa0fdb51f7aeec5e1fbc67aadd` |
| 4 | Seed phase, page `336`, after clicking `Undo last change` | The DOM no longer exposed the recovery banner or undo control; the task projection was `Verifying`. The rendered transcript visibly recorded `Undid last turn: removed rendered-recovery-probe.txt`. | The probe path was absent. The historical `Changed files (1)` summary remained, which is the prior-turn history rather than evidence that the file still exists. | `PASS` | `UNRECORDED` (action timestamp not exposed) | `a3e831d00d590baa0fdb51f7aeec5e1fbc67aadd` |
| 5 | Reload phase, fresh page `337`, after process restart | The page loaded the same project, task, prior prompt, and mutation history. The task remained `Verifying`, `Changed files (1)` remained as history, and no stale recovery banner or undo control was exposed. | The probe path remained absent after the fresh process loaded the durable state. | `PASS` | `UNRECORDED` (action timestamp not exposed) | `a3e831d00d590baa0fdb51f7aeec5e1fbc67aadd` |

## Acceptance status

| Acceptance row | Status | Evidence |
| --- | --- | --- |
| Safe-mode permission is rendered before the contained write | `CONFIRMED` | Seed DOM and inline permission capture on page `336`. |
| Allow performs only the contained fixture mutation | `CONFIRMED` | Page `336` showed one changed file; the workspace contained the expected 26-byte probe. |
| Undo restores the workspace and removes user-visible undo availability | `CONFIRMED` | Page `336` post-undo DOM/screenshot and absent probe path. |
| Fresh process reload preserves durable history without stale undo | `CONFIRMED` | Page `337` direct DOM/screenshot, reload manifest, and absent probe path. |
| Exact clean source provenance | `CONFIRMED` | Both manifests verified the same exact SHA and `source_tree_modified=false`. |
| Durable screenshot artifact | `UNRECORDED` | Inline screenshots were inspected, but no persisted path was returned. |
| Live-provider quality | `UNVERIFIED` | The fixture intentionally uses a deterministic local provider. |
| Cross-platform rendered behavior | `UNVERIFIED` | This run was macOS-only. |
| Arbitrary hostile filesystem races | `UNVERIFIED` | This run uses the contained deterministic fixture and does not exercise hostile writers. |
| Release readiness | `UNVERIFIED` | This evidence is one bounded rendered lane, not a release audit. |

## Validation

Focused tests passed on the fixture source tree:

```text
go test -tags rendered_fixture ./internal/gui -run 'TestRenderedRecoveryFixture|TestEmbeddedIndex' -count=1
ok  github.com/saiaathishkarthik/picogent/internal/gui  1.383s

go test ./internal/gui -run '^TestEmbeddedIndex$' -count=1
ok  github.com/saiaathishkarthik/picogent/internal/gui  0.545s
```

The automated rendered recovery test remains API-boundary evidence only; the
BrowserOS sequence above is the direct DOM evidence for this record. Neither
replaces live-provider, unsupported-platform, hostile-writer, or final release
audits.

## Reproduction outline

1. Build the rendered fixture from a clean checkout at the exact source SHA with
   `PICOGENT_RENDERED_FIXTURE_SOURCE_SHA` set to that SHA.
2. Run the seed phase from [the fixture runbook](V4-RENDERED-RECOVERY-FIXTURE.md)
   in Safe mode and approve only the contained write.
3. Verify the mutation, click `Undo last change`, and corroborate that the probe
   is absent.
4. Stop the seed process and run the reload phase against the same disposable
   home and workspace.
5. Record only directly observed DOM/UI text and keep unsupported boundaries
   `UNVERIFIED` or `UNRECORDED`.
