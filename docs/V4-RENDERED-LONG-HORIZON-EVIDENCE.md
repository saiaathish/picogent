# v4 rendered long-horizon direct evidence

This evidence record belongs to [#370](https://github.com/saiaathish/picogent/issues/370),
the large lane under [#366](https://github.com/saiaathish/picogent/issues/366). It records
one direct, task-owned Darwin Playwright Chromium observation of the merged rendered
long-horizon fixture at current `main` tip. It is evidence only; it is not a
live-provider, unsupported-platform, or release-readiness claim.

Historical BrowserOS neo observation at `993258f4…` and the `#550` package at
`22b1b1b…` remain on record below as superseded provenance; tip claims must use
the tip-bound section at `f8a78c6…`.

## Tip provenance (`f8a78c6…`)

- Observed source / exact `origin/main`:
  `f8a78c646877164009953401772acdb4d346175d` (`#551` behavior tip).
- `#551` changed `internal/ctxmgr/retention.go` and `internal/session/session.go`;
  prior `#550` long-horizon tip package at `22b1b1b…` is **STALE** and must not
  be retained via docs-only / behavior-SHA continuity.
- Runtime: `go-build-tags-rendered_fixture` with `vcs.revision=f8a78c6…`,
  `vcs.modified=false` (clean clone build).
- Fixture scenario: `rendered-multi-turn-outcome` / `-scenario long-horizon`.
- Fixture session: `rendered-long-horizon-fixture`.
- Browser: Playwright Chromium headless `151.0.7922.34` (darwin/arm64).
- Seed started: `2026-09-07T10:34:37.716617Z`.
- Reload started: `2026-09-07T10:34:45.792603Z`.
- Both manifests: `source_sha_verified=true`, `source_tree_modified=false`.

### Digests

```text
observation     3d1b2bde474e6207529272f6ba0c549940c02021005b9c2461e69f583d303796
screenshot-set  acf5dd11fade1454056d4214dda1dbc13c7412550fcdb86e1fe57f309621124a
seed-manifest   8e4504e903141f9d29fed1a3b72b0da3817257e1560497c68bda7d24e867baf4
reload-manifest 1da12a87cc4eab59f9a23c9f44038d16795cd369b68066c8d7a06cecd9c0da4e
```

Artifacts (outside checkout): `/private/tmp/picogent-evidence-f8a78c6/long-horizon/darwin/`.

Companion Darwin rendered-recovery + exact-head matrix notes:
[V4-RENDERED-RECOVERY-EVIDENCE-F8A78C6.md](V4-RENDERED-RECOVERY-EVIDENCE-F8A78C6.md)
and [V4-TIP-EVIDENCE-F8A78C6.md](V4-TIP-EVIDENCE-F8A78C6.md).

## Tip direct browser observations

| Sequence | Prompt / boundary | Directly observed UI | Outcome and freshness signal |
| --- | --- | --- | --- |
| 1 | Initial seed page | Safe-mode fixture session loaded without undo. | Baseline before mutation. |
| 2 | `Create the rendered UI outcome probe` after Safe-mode allow | Probe file present; completion proof pending; mutation staged. | Contained write succeeded; completion remained fail-closed. |
| 3 | `Verify the rendered UI outcome probe` | `PASS · verify PASS — deterministic workspace observation` visible while quality evidence still incomplete. | Deterministic verify did not promote completion. |
| 4 | `Review the rendered UI outcome after steering its scope` | Steering text and stale-proof / `INCONCLUSIVE` signals; `Undo last change` enabled. | Steering invalidated prior proof; undo remained available. |
| 5 | Fresh-process reload | Same durable fixture session retained history / blocked or proof-pending completion gate. | Restart/resume preserved fail-closed gate. |
| 6 | `Verify the rendered UI outcome after reload` | Fresh rendered inspection / `INCONCLUSIVE` / fail-closed language. | Reload did not reuse stale completion proof. |

Verdict `PASS` for this bounded Darwin owned-browser package at `f8a78c6…`.

## Interpretation

- Tip-bound rendered surface preserved mutation visibility, verification,
  steering invalidation, undo availability, and fresh-process reload recovery.
- Steering invalidated earlier proof; reload did not promote stale proof.
- Live-provider quality, unsupported platforms, hostile writers, broader crash
  windows, and release authorization remain `UNVERIFIED`.
- Tip `rendered-cross-platform` remains **STALE/UNVERIFIED** until Linux +
  Windows are rebound at `f8a78c6…`.

## Superseded tip package (`22b1b1b…` / `#550`)

Prior tip long-horizon package at `22b1b1ba7e1aec45d57172b6be480297e57637de`
(observation `6c6159cc…`, artifacts under
`/private/tmp/picogent-evidence-long-horizon-22b1b1b/darwin/`) landed immediately
before `#551` behavior changes and is **not** tip-bound after `f8a78c6…`.

## Historical observation (superseded; `993258f4…`)

Prior BrowserOS neo observation against
`993258f4b97d196fd7c44cca78c235080fd062e9` (2026-09-03) exercised the same
create → verify → steer → reload sequence with
`source_sha_verified: true`. That package is not tip-bound to current `main`
(`142` non-`docs/` paths changed before behavior tip `423d047…`) and must not be
cited as current tip proof.
