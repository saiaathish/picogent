# v4 rendered recovery evidence at tip e7c5f04

This record belongs to [#497](https://github.com/saiaathish/picogent/issues/497)
under parent [#453](https://github.com/saiaathish/picogent/issues/453). It
refreshes the direct BrowserOS neo allow→undo→reload observation against exact
merged `main` tip `e7c5f04d227c12eede0456d11a5ddcbb6636cee4`.

It does not claim live-provider quality by itself, cross-platform rendered
behavior, hostile-writer safety, or release readiness.

## Provenance

- Source SHA: `e7c5f04d227c12eede0456d11a5ddcbb6636cee4`.
- Fixture binary identity: `vcs.revision=e7c5f04d227c12eede0456d11a5ddcbb6636cee4`,
  `vcs.modified=false`.
- Runtime: `go-build-tags-rendered_fixture`.
- Host: `darwin/arm64`.
- Browser session: `cursor/rendered-recovery-tip`.
- Browser tab: task-owned BrowserOS neo page `341`.
- Seed URL: `http://127.0.0.1:49555/`.
- Reload URL: `http://127.0.0.1:49556/`.
- Fixture session: `rendered-recovery-fixture`.
- Seed started at `2026-09-06T12:04:06.518516Z`; reload started at
  `2026-09-06T12:05:51.680599Z`.
- Both manifests recorded `source_sha_verified=true` and
  `source_tree_modified=false`.
- Probe content SHA-256
  `6edfb9937f622dadd7cb093d2e4150c3747de904ef6be0e15d15eed69e1e627b`.
- Fixture manifests retain `issue=467` as the fixture implementation lane; this
  evidence checkpoint belongs to `#497`.

Author-host-local disposable fixture home, manifests, and matrix artifact paths
are not durable PR artifacts. Digests below are retained for review.

## Direct rendered observations

| Sequence | Boundary and direct UI observation | Local corroboration | Verdict |
| --- | --- | --- | --- |
| 1 | Fresh seed page loaded in Safe mode with no Undo control. | Seed workspace started without the probe. | `PASS` |
| 2 | Submitted task rendered Deny / This turn / Always allow / Allow while the probe remained absent. | Permission card observed before mutation. | `PASS` |
| 3 | Allow rendered `Edited 1 file`, `Changed files (1)`, and enabled `Undo last change`. | Expected probe content digest is the fixture contract hash above. | `PASS` |
| 4 | Undo removed the Undo control; probe path absent afterward. | Workspace probe path absent after undo. | `PASS` |
| 5 | Fresh-process reload preserved `Changed files (1)` history with no stale Undo. | Probe remained absent after reload. | `PASS` |

Screenshots were inspected inline and intentionally left `UNRECORDED`.

## Digest-only matrix artifact

```json
{
  "schema": "picogent.v4.rendered-platform-evidence.v1",
  "candidate_sha": "e7c5f04d227c12eede0456d11a5ddcbb6636cee4",
  "platform": "darwin",
  "architecture": "arm64",
  "environment": "task-owned-disposable",
  "browser": "browseros-neo",
  "fixture": "rendered-recovery",
  "observation_sha256": "ff192fc811d263b2fdad30838f4dae45f1efc17d5af204b9b8bd5c67e987830a",
  "screenshot_sha256": "UNRECORDED",
  "observed_at": "2026-09-06T12:06:55Z",
  "verdict": "PASS",
  "source_tree_modified": false
}
```

Artifact SHA-256:

```text
46a38b5b9c361dee5d9366023f1dc41cb29de92ec49743410782962f1d4eca92
```

## Matrix result with tip-bound live + rendered artifacts

```text
HEAD=PASS  tree=CLEAN
PASS: 8  INCONCLUSIVE: 1  UNVERIFIED: 2
rendered-platform-local=PASS
live-provider-connectivity=PASS
live-provider-quality=PASS
rendered-cross-platform=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

## Acceptance boundaries

- `CONFIRMED`: Safe permission before contained mutation.
- `CONFIRMED`: Allow performs the contained probe mutation.
- `CONFIRMED`: Undo restores workspace and removes Undo control.
- `CONFIRMED`: fresh-process reload preserves history without stale Undo.
- `PASS`: local rendered-platform matrix row for `darwin/arm64`.
- `UNVERIFIED`: Windows/Linux rendered behavior and cross-platform claims.
- `UNVERIFIED`: arbitrary same-UID hostile filesystem TOCTOU.
- `INCONCLUSIVE`: release authorization.
