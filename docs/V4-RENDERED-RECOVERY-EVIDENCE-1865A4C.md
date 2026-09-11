# v4 rendered recovery evidence at behavior candidate `1865a4c` (Darwin)

This record belongs to [#492](https://github.com/saiaathish/picogent/issues/492),
the small direct-observation child under
[#453](https://github.com/saiaathish/picogent/issues/453). It records one
task-owned BrowserOS neo observation of the real embedded GUI on
`darwin/arm64`, bound to the exact clean behavior candidate
`1865a4c00356ac9b871b35b3a08f4e983f2af0e1`.

This is bounded evidence for `rendered-platform-local`. It does not claim
cross-platform rendered behavior, arbitrary same-UID filesystem TOCTOU safety,
live-provider recovery, or release authorization.

## Provenance

- Behavior candidate: `1865a4c00356ac9b871b35b3a08f4e983f2af0e1`.
- Source verification: `vcs.revision` matched the candidate and
  `vcs.modified=false`.
- Runtime: `go-build-tags-rendered_fixture`.
- Host: `darwin/arm64`.
- Browser: BrowserOS neo, task-owned session `codex/rendered-v4-qa`; one
  task-owned tab was used for both phases and existing user tabs were left
  untouched.
- Fixture: `rendered-recovery`, session `rendered-recovery-fixture`.
- Seed URL: `http://127.0.0.1:51326/`.
- Reload URL: `http://127.0.0.1:51451/`.
- Fixture home and workspace were fresh disposable paths below the operating
  system temporary directory and are not retained in this PR:
  `/var/folders/z_/zxn_ghn96dd_78qxc_dfh9_00000gq/T/tmp.SpEwAVigXZ` and
  `/var/folders/z_/zxn_ghn96dd_78qxc_dfh9_00000gq/T/tmp.SpEwAVigXZ/workspace`.
- Seed and reload manifests were written inside that disposable home and
  recorded `source_sha_verified=true`, `source_tree_modified=false`,
  `before=absent`, and the applied probe SHA-256
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.
- Digest-only rendered artifact: `/private/tmp/picogent-rendered-platform-1865a4c.json`,
  SHA-256 `7c338f78e3ede6f017e17c53b2016596c53ca0f0fd9a6e001d7ed8fdf4a04ad2`.
  It contains no URLs, DOM, screenshots, credentials, or browser transcript.
- Artifact observation timestamp: `2026-09-11T02:15:51Z`. Individual browser
  action timestamps and a durable screenshot path were not exposed and remain
  `UNRECORDED`.

## Direct rendered observations

| Sequence | Boundary and direct UI observation | Local corroboration | Verdict |
| --- | --- | --- | --- |
| 1 | The fresh seed page loaded in Safe mode with no Undo control. | The seed manifest recorded the probe as initially absent. | `PASS` |
| 2 | Submitting `Create the rendered recovery probe file` rendered the Safe permission card with `Deny`, `This turn`, `Always allow`, and `Allow`; the probe was not yet present. | The fixture began from the fresh contained workspace. | `PASS` |
| 3 | Clicking `Allow` rendered `Edited 1 file`, `Changed files (1)`, and an enabled `Undo last change` control. | The probe existed at 26 bytes with SHA-256 `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`. | `PASS` |
| 4 | Clicking `Undo last change` removed the recovery control and rendered `Undid last turn: removed rendered-recovery-probe.txt`. | The probe path was absent. | `PASS` |
| 5 | Navigating the same task-owned tab to the reload URL after stopping the seed process preserved the durable task/history, with no live Undo control or permission card. | Reload API state reported `busy=false`, `pending_perm=null`, `undo_available=false`; the task remained `verifying` with historical `changed_files=[rendered-recovery-probe.txt]`; the probe remained absent. | `PASS` |

The `Changed files (1)` value after Undo and after reload is retained task
history. It is not evidence that the undone path still exists. The durable
transcript retains its earlier `Undo: /undo` text, while the fresh process did
not expose a live Undo control; those are distinct states.

## Matrix binding

The repository runtime-boundary matrix accepted the digest-only artifact using
the docs-only continuity contract:

- Matrix candidate: `3579b4a2fa2c74f73e3f4c3c0c77802b13f2cc13`.
- Behavior SHA: `1865a4c00356ac9b871b35b3a08f4e983f2af0e1`.
- Behavior provenance: `DOCS_ONLY_DESCENDANT`.
- Head match / tree: `PASS` / `CLEAN`.
- Retained matrix: `/private/tmp/picogent-quality-1865a4c/runtime-matrix-rendered-darwin.json`.
- Retained matrix SHA-256: `e26b468b13f3de33562afbd9de4dc58031757d7f456639b435b378ddefed204b`.
- Summary: `PASS 9 / INCONCLUSIVE 1 / UNVERIFIED 2`.

The matrix rows remain independently bounded:

| Claim | Verdict | Boundary |
| --- | --- | --- |
| `rendered-platform-local` | `PASS` | Direct task-owned Darwin rendered recovery observation recorded above. |
| `rendered-cross-platform` | `UNVERIFIED` | No matching Linux/Windows aggregate was supplied for this candidate. |
| `hostile-filesystem-toctou` | `UNVERIFIED` | Broad arbitrary same-UID races remain outside this fixture. |
| `release-authorization` | `INCONCLUSIVE` | No human release decision was recorded; this evidence does not authorize release. |

## Acceptance boundaries

- `CONFIRMED`: Safe permission renders before the contained write.
- `CONFIRMED`: Allow performs the expected one-file contained mutation.
- `CONFIRMED`: Undo restores the workspace and removes user-visible Undo.
- `CONFIRMED`: A fresh process reloads durable history without stale live
  recovery state.
- `CONFIRMED`: The rendered-platform artifact is bound to an exact clean
  behavior SHA and validates through the matrix loader.
- `UNVERIFIED`: rendered behavior on Linux or Windows, arbitrary hostile
  same-UID filesystem TOCTOU, durable screenshot retention, and
  live-provider recovery quality.
- `INCONCLUSIVE`: overall release authorization.

This is a rendered macOS checkpoint, not a v4 completion or release claim.
