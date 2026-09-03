# v4 rendered long-horizon direct evidence

This evidence record belongs to [#370](https://github.com/saiaathish/picogent/issues/370), the large lane under [#366](https://github.com/saiaathish/picogent/issues/366). It records one direct, task-owned BrowserOS neo observation of the merged rendered fixture. It is evidence only; it is not a live-provider, unsupported-platform, or release-readiness claim.

## Provenance

- Observed source and exact `origin/main`: `993258f4b97d196fd7c44cca78c235080fd062e9`.
- Runtime: `go-build-tags-rendered_fixture`.
- Fixture scenario: `rendered-multi-turn-outcome`.
- Fixture session: `rendered-long-horizon-fixture`.
- Browser session: `codex/rendered-evidence`, task-owned page `127`.
- Seed manifest: `/var/folders/z_/zxn_ghn96dd_78qxc_dfh9_00000gq/T/picogent-rendered-long-horizon-home-1579389420/rendered-long-horizon-fixture-seed.json`.
- Reload manifest: `/var/folders/z_/zxn_ghn96dd_78qxc_dfh9_00000gq/T/picogent-rendered-long-horizon-home-1579389420/rendered-long-horizon-fixture-reload.json`.
- Both manifests recorded `source_sha_verified: true` and `source_tree_modified: false`.
- Seed started at `2026-09-03T03:51:13.078857Z`; reload started at `2026-09-03T03:54:55.136277Z`.
- The screenshot API returned direct inline captures in the owned browser session; it did not expose a persisted filesystem path. Screenshot path: `UNRECORDED`.

The fixture home and workspace were disposable and shared only between the seed and reload phases. The deterministic provider was used intentionally; no live-provider behavior was exercised.

## Direct browser observations

The browser followed the documented prompts in Safe mode. Each row is a direct DOM/UI observation from the task-owned page, not an inference from source tests.

| Sequence | Prompt / boundary | Directly observed UI | Outcome and freshness signal |
| --- | --- | --- | --- |
| 1 | `Create the rendered UI outcome probe` after Safe-mode mutation approval | Progress showed `Blocked`, `3 of 4 steps`, and `Completion proof pending: durable task is blocked`; the UI showed `Blocked: verification inconclusive`, the changed file `rendered-long-horizon-probe.txt`, and an `Undo last change` control. | `INCONCLUSIVE · verify INCONCLUSIVE — rendered inspection is pending`; the contained mutation was staged while proof remained pending. |
| 2 | `Verify the rendered UI outcome probe` after verifier approval | Progress remained proof-gated; the rendered history showed the prior inconclusive result and the durable task update failure requiring current proof. | `PASS · verify PASS — deterministic workspace observation` was visible, while one quality requirement remained missing. The deterministic pass did not create direct rendered proof. |
| 3 | `Review the rendered UI outcome after steering its scope` after verifier approval | The UI showed the steering message and `Steering changed the outcome contract; earlier proof is stale.` The task returned to `Blocked: verification inconclusive`. | `INCONCLUSIVE · verify INCONCLUSIVE — fresh rendered inspection is required`; prior verification was not reused after steering. |
| 4 | Fresh reload before sending a new prompt | The new process displayed the same task, prior prompt history, changed file, and `Blocked` progress state. | The durable task and transcript crossed the process boundary; the rendered gate stayed fail-closed. |
| 5 | `Verify the rendered UI outcome after reload` after verifier approval | The reloaded page continued to show `Blocked`, `3 of 4 steps`, `Completion proof pending: durable task is blocked`, and the retained prior history. | `INCONCLUSIVE · verify INCONCLUSIVE — fresh rendered inspection is required`; fresh rendered proof was still required after reload. |

## Interpretation

- The direct rendered surface preserved task identity, progress, mutation visibility, verification history, steering invalidation, undo availability, and reload recovery.
- The surface did not claim completion from a deterministic workspace pass alone.
- Steering invalidated earlier proof, and reload did not promote stale proof.
- The exact source/runtime boundary is verified for this run, but the evidence remains bounded to the local fixture and one owned browser session.
- Live-provider quality, arbitrary hostile writers, unsupported platforms, broader crash windows, and v4 release readiness remain `UNVERIFIED`.

## Reproduction outline

1. Build the fixture with VCS stamping enabled from a clean checkout at the exact source SHA.
2. Run `go run -tags rendered_fixture ./cmd/picogent-rendered-fixture -scenario long-horizon` with `PICOGENT_RENDERED_FIXTURE_SOURCE_SHA` set to the source SHA.
3. Follow the four seed prompts in [the fixture runbook](V4-RENDERED-LONG-HORIZON-FIXTURE.md), approving only the contained Safe-mode fixture permissions.
4. Stop the seed process and run the reload phase with the manifest's disposable home and workspace.
5. Record only directly observed DOM/UI text and keep unsupported boundaries `UNVERIFIED` or `UNRECORDED`.
