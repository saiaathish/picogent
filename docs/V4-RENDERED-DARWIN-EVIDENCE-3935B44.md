# v4 Darwin rendered recovery evidence at exact candidate `3935b44`

This record belongs to [#453](https://github.com/saiaathishkarthik/picogent/issues/453).
It records one fresh task-owned BrowserOS neo observation of the real embedded
GUI on `darwin/arm64`, bound to the exact clean candidate
`3935b4482f703662acff21f23b205f0763ee1758`.

This is bounded evidence for `rendered-platform-local`. It does not claim
Linux or Windows rendered behavior, arbitrary same-UID filesystem TOCTOU
safety, live-provider recovery quality, or release authorization. The
candidate is a documentation-only descendant of behavior tip
`9c1ca2e4df5af35dde5ed5e1f5cce39368e9b2fb`; the rendered run itself was
collected and checked at the exact current candidate.

## Provenance

- Candidate: `3935b4482f703662acff21f23b205f0763ee1758`.
- Source verification: the fixture reported `vcs.revision` equal to the
  candidate and `vcs.modified=false`.
- Host/runtime: `darwin/arm64` with the `rendered-recovery` fixture.
- Browser: BrowserOS neo, task-owned session `codex/rendered-evidence`; the
  observation used a task-owned tab.
- Seed started at `2026-09-11T09:50:40.009037Z`; reload started at
  `2026-09-11T09:53:55.036315Z`.
- The seed and reload fixture processes exited cleanly. Their disposable
  fixture home, workspace, source clone, and binaries were moved to the user's
  Trash after collection and are not retained in the checkout.
- The digest-only platform artifact is
  `/private/tmp/picogent-rendered-current-darwin/darwin-rendered-platform-evidence.json`;
  its SHA-256 is
  `c1a480bb76879cc8595e58abc7b540a0d3591d38e8c43b1924f1705796cedfed`.
- The observation record is
  `/private/tmp/picogent-rendered-current-darwin/darwin-rendered-recovery-observation.json`;
  its SHA-256 is
  `5d64b6bb6f9c53c3ceea79282bb61a46cdfde531dbc6c3ca4e4e12369c16eaa7`.
- The exact-head runtime matrix is
  `/private/tmp/picogent-rendered-current-darwin/runtime-boundary-matrix.json`;
  its SHA-256 is
  `0313eb667c1cffbcb9783dbeb2b18f5998157da02f204e1f7cc3bc65b724bc95`.
- Seed manifest SHA-256:
  `006cdc4ea85ef30a64e4192ff5c482f53775d54b20c17d2c2d0a10f59a4da04b`.
- Reload manifest SHA-256:
  `6e296a2d1d945d634a92a184e8bc7a68ec9789180e38030cc7178ceb3e7d9f9d`.
- The observation and platform records report `observed_at=2026-09-11T09:56:24Z`,
  `verdict=PASS`, and `screenshot_sha256=UNRECORDED`; no screenshot bytes are
  retained.

## Direct rendered observations

| Sequence | Direct UI observation | Local corroboration | Verdict |
| --- | --- | --- | --- |
| 1 | A fresh Safe page loaded without an Undo control. | The probe was absent before Allow. | `PASS` |
| 2 | Submitting the probe request rendered the Safe permission card with `Deny`, `This turn`, `Always allow`, and `Allow`; the probe remained absent. | The fixture began from the fresh contained workspace. | `PASS` |
| 3 | Clicking `Allow` rendered `Edited 1 file`, `Changed files (1)`, and an Undo control. | The probe was present with SHA-256 `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`. | `PASS` |
| 4 | Clicking Undo removed the probe and the Undo control. | The probe path was absent; retained `Changed files (1)` is history, not a live path assertion. | `PASS` |
| 5 | Reloading the fresh process in the same task-owned tab preserved durable history without a stale Undo control or permission card. | Reload state recorded `busy=false`, `pending_perm=null`, `undo_available=false`; the probe remained absent and fixture processes exited. | `PASS` |

## Matrix binding

The retained exact-head runtime matrix accepted the rendered artifact with:

- Candidate / behavior SHA: `3935b4482f703662acff21f23b205f0763ee1758` /
  `3935b4482f703662acff21f23b205f0763ee1758`.
- Behavior provenance: `EXACT_HEAD`.
- HEAD / tree: `PASS` / `CLEAN`.
- Host: `darwin/arm64`.
- Summary: `PASS 7 / INCONCLUSIVE 1 / UNVERIFIED 4`.
- `rendered-platform-local`: `PASS`.

The exact-head snapshot intentionally supplied no live-provider artifacts, so
its live-provider rows remain fail-closed `UNVERIFIED`; this does not erase the
earlier behavior-bound live-provider continuity packet documented in the
runtime matrix. The rendered refresh changes only the local rendered claim:

| Claim | Verdict | Boundary |
| --- | --- | --- |
| `rendered-platform-local` | `PASS` | Direct task-owned Darwin/arm64 recovery observation at the exact clean candidate. |
| `rendered-cross-platform` | `UNVERIFIED` | No matching Linux/Windows aggregate was supplied for this candidate. |
| `hostile-filesystem-toctou` | `UNVERIFIED` | Broad arbitrary same-UID filesystem races remain outside this fixture. |
| `release-authorization` | `INCONCLUSIVE` | No operator release decision was recorded; this evidence does not authorize release. |

## Acceptance boundaries

- `CONFIRMED`: Safe permission renders before the contained write.
- `CONFIRMED`: Allow performs the expected one-file contained mutation.
- `CONFIRMED`: Undo restores the workspace and removes user-visible Undo.
- `CONFIRMED`: A fresh process reloads durable history without stale live
  recovery state.
- `CONFIRMED`: The digest-only rendered artifact is bound to the exact clean
  candidate and validates through the matrix loader.
- `UNVERIFIED`: rendered behavior on Linux or Windows, arbitrary hostile
  same-UID filesystem TOCTOU, live-provider recovery quality, and durable
  screenshot retention.
- `INCONCLUSIVE`: overall release authorization.

This is a rendered macOS checkpoint, not a v4 completion or release claim.
