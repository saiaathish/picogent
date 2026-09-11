# V4 current-main residual packet at `166b7d3`

Status: **exact-head audit refresh only**. This packet records the current
fail-closed posture for parent issue [#453](https://github.com/saiaathish/picogent/issues/453).
It does not authorize a release, sign the residual-acceptance stub, close the
parent issue, or claim v4 completion.

## Exact-head binding

| Field | Value |
| --- | --- |
| Candidate SHA | `166b7d37d3f5b07de389741110fa62a48bb6f65e` |
| Candidate state | clean `main`, `HEAD=PASS`, `tree=CLEAN` |
| Behavior provenance | `EXACT_HEAD` |
| Host | `darwin/arm64`, `go1.26.6` |
| Generated at | `2026-09-11T22:32:24Z` |
| Matrix artifact | `/private/tmp/picogent-current-main-residual/runtime-boundary-matrix.json` |
| Matrix SHA-256 | `66516803f5bec85084ff30dc4296ba661dee3c19372f91bc1a25aa4ed8d1c8d8` |
| Matrix summary | `PASS 6 / INCONCLUSIVE 1 / UNVERIFIED 5` |

The artifact was generated without live-provider or rendered-platform
environment inputs. Their absence is intentional in this refresh and is
reported fail-closed by the matrix rather than inferred from older packets.

## Current claim posture

| Claim | Verdict | Boundary |
| --- | --- | --- |
| `hostile-child-env-sanitization` | `PASS` | Bounded deterministic child-environment evidence. |
| `hostile-filesystem-deterministic` | `PASS` | Bounded deterministic securefile/procenv/workspace evidence. |
| `hostile-parent-swap-confinement` | `PASS` | Bounded Darwin/Linux parent-swap confinement; not universal TOCTOU proof. |
| `rendered-long-horizon-local` | `PASS` | Existing local fixture contract; not current browser or cross-platform proof. |
| `rendered-recovery-undo-reload` | `PASS` | Deterministic API-boundary fixture; not browser DOM proof. |
| `restart-steer-undo-recovery` | `PASS` | Provider-independent deterministic recovery contracts. |
| `hostile-filesystem-toctou` | `UNVERIFIED` | Broad same-UID races remain an explicit residual. |
| `live-provider-connectivity` | `UNVERIFIED` | No fresh `PICOGENT_LIVE_PROVIDER_EVIDENCE` artifact supplied. |
| `live-provider-quality` | `UNVERIFIED` | No fresh `PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE` artifact supplied. |
| `rendered-cross-platform` | `UNVERIFIED` | No fresh `PICOGENT_RENDERED_CROSS_PLATFORM_EVIDENCE` artifact supplied. |
| `rendered-platform-local` | `UNVERIFIED` | No fresh `PICOGENT_RENDERED_PLATFORM_EVIDENCE` artifact supplied. |
| `release-authorization` | `INCONCLUSIVE` | Required live/rendered evidence is absent and no operator approval is supplied. |

The bounded Windows parent-replacement records and the TOCTOU boundary
inventory remain separately documented in
[V4-TOCTOU-BOUNDARY-INVENTORY.md](V4-TOCTOU-BOUNDARY-INVENTORY.md). They do
not upgrade the universal residual row.

## Operator decision

- Residual acceptance: **unsigned / absent**.
- Operator approval for `v4-release`: **absent**.
- Publish, tag, deploy, issue closure, and goal completion: **not authorized**.
- The correct parent posture is **OPEN**, with the residual and missing live or
  rendered packets retained explicitly rather than synthesized from historical
  evidence.

## Reproduction

From a clean checkout at the candidate SHA:

```sh
GOMAXPROCS=2 GOFLAGS=-p=1 GOTOOLCHAIN=local \
  go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha 166b7d37d3f5b07de389741110fa62a48bb6f65e \
  --out /private/tmp/picogent-current-main-residual/runtime-boundary-matrix.json
```

The packet is a current-point refresh. A later non-documentation behavior
change requires fresh evidence; a docs-only descendant must not silently turn
the absent packets or residual into a `PASS` claim.
