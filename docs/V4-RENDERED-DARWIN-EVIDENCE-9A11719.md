# v4 Darwin rendered recovery evidence at exact current main `9a11719`

This record belongs to [#453](https://github.com/saiaathishkarthik/picogent/issues/453).
It records one fresh task-owned BrowserOS neo observation of the real embedded
GUI on `darwin/arm64`, bound to the exact clean `main` tip
`9a1171942e9041a9e25a75e30e6d9e5dc7d7ad5e` before this documentation-only
checkpoint.

The observation is bounded to the named Darwin rendered-recovery fixture. It
does not claim Linux or Windows rendered behavior at this candidate,
cross-platform coverage, live-provider quality, arbitrary hostile same-UID
filesystem races, or release authorization.

## Provenance

- Candidate: `9a1171942e9041a9e25a75e30e6d9e5dc7d7ad5e` (`main` before this
  docs-only checkpoint).
- Source checkout: clean exact-SHA clone; `HEAD` matched the candidate and the
  source tree was clean.
- Fixture build: Go `go1.26.6`, `-tags rendered_fixture`, with
  `vcs.revision=9a1171942e9041a9e25a75e30e6d9e5dc7d7ad5e` and
  `vcs.modified=false`. The verified binary was built with `-buildvcs=true`;
  Go `go run` omitted VCS settings on this host, so it was not used for the
  provenance-bound observation.
- Host/runtime: `darwin/arm64`.
- Browser: BrowserOS neo, task-owned session `codex/recovery-proof`,
  task-owned page `14`.
- Fixture session: `rendered-recovery-fixture`.
- Seed URL: `http://127.0.0.1:58380/`.
- Reload URL: `http://127.0.0.1:58579/`.
- Seed started at `2026-09-11T15:01:22.710436Z` in PID `80717`; reload started
  at `2026-09-11T15:05:59.896953Z` in PID `81122`. Both processes were
  verified exited after their phase.
- Author-host-local disposable fixture home, workspace, and manifests were
  outside the checkout under
  `/var/folders/z_/zxn_ghn96dd_78qxc_dfh9_00000gq/T/picogent-rendered-recovery-home-4105540882/`;
  they are not PR artifacts.
- Seed manifest SHA-256:
  `ea4181d3ae1231a9554588ea9fa163474e7928d8a0308ad7f32af99996bc6f3a`.
- Reload manifest SHA-256:
  `c8bfdc31900cc220457b68143736f7658c8e46edf3051fd6246c42e660e0bd99`.
- Both manifests recorded the exact candidate SHA,
  `source_sha_verified=true`, `source_tree_modified=false`, and the probe
  content SHA-256
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.
- The secret-free observation artifact was
  `/private/tmp/picogent-rendered-current-9a11719/darwin-rendered-recovery-observation.json`;
  SHA-256
  `e43f4c0d450d4a17f93a15e26a1242607dc55a6f115da7514fb07d71143147ef`.
- The local platform artifact was
  `/private/tmp/picogent-rendered-current-9a11719/darwin-rendered-platform-evidence.json`;
  SHA-256
  `517ebe333f3b80ade0471f94100acabb1b7a4b96e17719255706072fd6bdb94c`.
- The exact-head runtime matrix was
  `/private/tmp/picogent-rendered-current-9a11719/runtime-boundary-matrix.json`;
  SHA-256
  `c2e11de481c22087d9ac7ca9c8c7db1b40c61b2739d5eccfd9ac6f6d00c1baba`.
- The matrix was generated at `2026-09-11T15:09:30Z`. Browser action
  timestamps and durable screenshot files were not exposed and are therefore
  `UNRECORDED`; screenshots were inspected inline at the permission,
  post-Allow, post-Undo, and reload checkpoints.

The fixture exercised the normal embedded GUI, Safe permission flow,
`/api/permission`, `/api/chat`, SSE updates, durable task/session state, and
checkpoint-backed Undo. Its provider was deterministic and local by design.

## Direct rendered observations

| Sequence | Direct UI observation | Local corroboration | Verdict |
| --- | --- | --- | --- |
| 1 | A fresh seed page loaded in Safe mode with the task empty and no Undo control. | The seed manifest recorded `before=absent` for the contained probe. | `PASS` |
| 2 | Submitting `Create the rendered recovery probe file` rendered the Safe permission card with `Deny`, `This turn`, `Always allow`, and `Allow`; the card explicitly identified `write rendered-recovery-probe.txt`, while the probe remained absent. | The disposable workspace was fresh before Allow. | `PASS` |
| 3 | Clicking `Allow` rendered `Edited 1 file`, `Changed files (1)`, and the enabled `Undo last change` control. | The probe was present with SHA-256 `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`. | `PASS` |
| 4 | Clicking `Undo last change` removed the recovery control and rendered `Undid last turn: removed rendered-recovery-probe.txt`. | The probe path was absent after Undo; the retained `Changed files (1)` is task history, not a live path assertion. | `PASS` |
| 5 | After the seed process stopped, the same task-owned page loaded the reload URL in a fresh process. Durable task/history rendered with `Verifying` and `Changed files (1)`, without a stale permission card or Undo control. | The reload manifest recorded a distinct PID and the probe remained absent. | `PASS` |

## Exact-head matrix result

The runtime-boundary matrix was run from the clean exact-SHA clone with the
Darwin platform artifact supplied:

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

The local `PASS` is only the `darwin/arm64` task-owned rendered row. The
matrix did not receive live-provider or cross-platform artifacts, and the
broad same-UID TOCTOU residual and release authorization remain unchanged.

## Acceptance boundaries

- `CONFIRMED`: Safe permission renders before the contained write.
- `CONFIRMED`: Allow performs the expected one-file contained mutation.
- `CONFIRMED`: Undo restores the workspace and removes user-visible Undo.
- `CONFIRMED`: A fresh process reloads durable task history without stale live
  recovery state.
- `CONFIRMED`: The fixture manifests bind the observation to the exact clean
  candidate through matching `vcs.revision` and `vcs.modified=false`.
- `PASS`: `rendered-platform-local` for Darwin/arm64 at candidate
  `9a1171942e9041a9e25a75e30e6d9e5dc7d7ad5e`.
- `UNVERIFIED`: Linux, Windows, and the exact-candidate cross-platform
  rendered claim.
- `UNVERIFIED`: live-provider claims, arbitrary hostile same-UID filesystem
  TOCTOU, and durable screenshot retention.
- `INCONCLUSIVE`: overall release authorization; this observation does not
  provide operator approval.

This is a current-main rendered checkpoint, not a v4 completion or release
claim.
