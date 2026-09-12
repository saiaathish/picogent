# V4 current exact-head residual packet at a82f846

Status: **NOT COMPLETE / exact-head evidence refresh**. This packet records a
fresh local runtime-boundary matrix from the merged session-lock hardening
candidate. It does not authorize a release, close parent #453, or claim v4
completion.

## Candidate and provenance

| Field | Value |
| --- | --- |
| Behavior candidate | a82f8463f9f936dcb93da83dd97485e25b5ac480 |
| Merged behavior PR | #719 |
| Evidence refresh issue | #720 |
| Parent issue | #453, OPEN |
| Head match | PASS |
| Tree | CLEAN |
| Behavior provenance | EXACT_HEAD |
| Host | darwin/arm64 |
| Generated at | 2026-09-12T05:29:16Z |
| Matrix schema | picogent.v4.runtime-boundary-matrix.v1 |
| Retained artifact | /private/tmp/picogent-v4-evidence-a82f846.GgRCPK/runtime-boundary-matrix.json |
| Artifact SHA-256 | efa9c0efe1ecb5c7a75e2b884fb181c843f378a52ac317346e149bcfb58bdcea |

The artifact was written outside the checkout with the matrix retention
contract. Its exact-head result was:

    HEAD=PASS
    tree=CLEAN
    behavior_provenance=EXACT_HEAD
    summary: UNVERIFIED=12

The focused regression command also passed at this candidate:

    GOMAXPROCS=2 GOFLAGS=-p=1 GOTOOLCHAIN=local go test ./internal/session -count=1

## Claim posture

The matrix intentionally supplied no live-provider, rendered-platform, or
retained hostile-runtime artifacts. It therefore reported all twelve claims
as UNVERIFIED rather than projecting older observations onto a new behavior
SHA.

| Claim group | Result | Evidence boundary |
| --- | --- | --- |
| hostile-child-env-sanitization; hostile-filesystem-deterministic | UNVERIFIED | Exact-head evidence documents are stale or absent for this refresh. |
| hostile-filesystem-toctou | UNVERIFIED | The broad arbitrary same-UID race residual remains explicitly unproved. |
| hostile-parent-swap-confinement | UNVERIFIED | No valid exact-head Darwin/Linux parent-swap packet was supplied. |
| live-provider-connectivity; live-provider-quality | UNVERIFIED | No exact-head provider artifacts were supplied. |
| rendered-cross-platform; rendered-long-horizon-local; rendered-platform-local; rendered-recovery-undo-reload | UNVERIFIED | No exact-head rendered artifacts were supplied. |
| restart-steer-undo-recovery | UNVERIFIED | Existing deterministic records are not silently rebound to this behavior SHA. |
| release-authorization | UNVERIFIED in this matrix | The exact-head release documents are stale or absent; the separate operator predicate remains INCONCLUSIVE with authorized=false. |

Historical live, rendered, hostile, and recovery packets remain separately
bound to their recorded behavior candidates. They are not unioned with this
matrix into a synthetic release-authorizing result.

## Session lock hardening covered by this checkpoint

PR #719 changed session List, ListMeta, Prune, and Delete to acquire the
shared session lock before probing directory state. Missing-directory
behavior remains unchanged. The deterministic List regression proves that a
directory probe waits for the session lock and observes the state after the
lock is released.

This narrows one session coordination edge. It does not make directory
enumeration, multi-record workflows, or every filesystem pathname boundary
an atomic cross-process snapshot, and it does not upgrade the broad
hostile-filesystem-toctou row.

## Release and parent boundary

The current formal posture remains **NOT COMPLETE**. Release authorization
still requires a separate human operator decision, and the broad hostile
TOCTOU residual remains UNVERIFIED. A later documentation-only descendant
may retain this exact-head packet under the repository continuity rule, but
must not silently rebind its evidence to the descendant SHA.

Parent #453 remains open. This packet records a bounded checkpoint only.
