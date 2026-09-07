# v4 rendered long-horizon direct evidence

This evidence record belongs to [#370](https://github.com/saiaathish/picogent/issues/370),
the large lane under [#366](https://github.com/saiaathish/picogent/issues/366). It records
one direct, task-owned Darwin Playwright Chromium observation of the merged rendered
long-horizon fixture at current `main` tip. It is evidence only; it is not a
live-provider, unsupported-platform, or release-readiness claim.

Historical BrowserOS neo observation at `993258f4…` remains on record below the tip
section as superseded provenance; tip claims must use the tip-bound section.

## Tip provenance (`22b1b1b…`)

- Observed source / exact `origin/main`:
  `22b1b1ba7e1aec45d57172b6be480297e57637de`.
- Behavior ancestor for prior tip packages:
  `423d0471c1864651473c2f1828b67066885f79bd` (docs-only path from that SHA to tip).
- Runtime: `go-build-tags-rendered_fixture` with `vcs.revision=22b1b1b…`,
  `vcs.modified=false` (clean detached clone build).
- Fixture scenario: `rendered-multi-turn-outcome` / `-scenario long-horizon`.
- Fixture session: `rendered-long-horizon-fixture`.
- Browser: Playwright Chromium headless `151.0.7922.34` (darwin/arm64).
- Seed started: `2026-09-07T09:25:21.24316Z`.
- Reload started: `2026-09-07T09:25:31.784094Z`.
- Both manifests: `source_sha_verified=true`, `source_tree_modified=false`.

### Digests

```text
observation     6c6159cc0be43d8c63ea64e68cd58f341d0bc0210b8d80944ec739b0643331be
screenshot-set  67ece978ea5f07f3643642c3a06402b8f3f119b0030deb72b05e7eeafe1f8e39
seed-manifest   fe55bd1c5cc6753e9c55624aa29862afa8a56da4111fc2346573d7189ce1738d
reload-manifest 1f9d0d9eb1aa71901bab4961206d31b52b143dd28c37bcee3c125571c9c620f2
```

Artifacts (outside checkout): `/private/tmp/picogent-evidence-long-horizon-22b1b1b/darwin/`.

BrowserOS neo could not reach host loopback (`127.0.0.1`) in this collection
environment; tip recollection used the same local Playwright path as tip Darwin
rendered-recovery packages. Deterministic companion proofs at the same tip:

```text
go test ./internal/agent -run '^(TestSteeringAcrossProcessRestart|TestLongHorizonResumeAfterProcessExit|TestLongHorizonResumeAfterProcessKill)$' -count=1
go test -tags rendered_fixture ./internal/gui -run '^(TestRenderedLongHorizonFixtureAPIBoundary|TestRenderedRecoveryFixtureAPIBoundary)$' -count=1
```

Both exited `0` on `22b1b1b…`.

## Tip direct browser observations

| Sequence | Prompt / boundary | Directly observed UI | Outcome and freshness signal |
| --- | --- | --- | --- |
| 1 | Initial seed page | Safe-mode fixture session loaded without undo. | Baseline before mutation. |
| 2 | `Create the rendered UI outcome probe` after Safe-mode allow | Probe file present; completion proof pending; mutation staged. | Contained write succeeded; completion remained fail-closed. |
| 3 | `Verify the rendered UI outcome probe` | `PASS · verify PASS — deterministic workspace observation` visible while quality evidence still incomplete. | Deterministic verify did not promote completion. |
| 4 | `Review the rendered UI outcome after steering its scope` | Steering text and stale-proof / `INCONCLUSIVE` signals; `Undo last change` enabled. | Steering invalidated prior proof; undo remained available. |
| 5 | Fresh-process reload | Same durable fixture session retained history / blocked or proof-pending completion gate. | Restart/resume preserved fail-closed gate. |
| 6 | `Verify the rendered UI outcome after reload` | Fresh rendered inspection / `INCONCLUSIVE` / fail-closed language. | Reload did not reuse stale completion proof. |

Verdict `PASS` for this bounded Darwin owned-browser package.

## Interpretation

- Tip-bound rendered surface preserved mutation visibility, verification,
  steering invalidation, undo availability, and fresh-process reload recovery.
- Steering invalidated earlier proof; reload did not promote stale proof.
- Live-provider quality, unsupported platforms, hostile writers, broader crash
  windows, and release authorization remain `UNVERIFIED`.

## Historical observation (superseded; `993258f4…`)

Prior BrowserOS neo observation against
`993258f4b97d196fd7c44cca78c235080fd062e9` (2026-09-03) exercised the same
create → verify → steer → reload sequence with
`source_sha_verified: true`. That package is not tip-bound to current `main`
(`142` non-`docs/` paths changed before behavior tip `423d047…`) and must not be
cited as current tip proof.
