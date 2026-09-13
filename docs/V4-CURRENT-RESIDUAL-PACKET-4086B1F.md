# V4 current exact-head residual packet at 4086b1f

Status: **NOT COMPLETE / exact-head evidence refresh**. This packet records a
fresh local runtime-boundary matrix from the merged evolution-store load-lock
hardening candidate. It does not authorize a release, close parent #453, or
claim v4 completion.

## Candidate and provenance

| Field | Value |
| --- | --- |
| Behavior candidate | `4086b1f3416cde476025718139aa2c8260cc9d8e` |
| Merged behavior PR | #723 (child #722) |
| Evidence refresh issue | #724 |
| Parent issue | #453, OPEN |
| Head match | PASS |
| Tree | CLEAN |
| Behavior provenance | EXACT_HEAD |
| Host | darwin/arm64 |
| Generated at | 2026-09-12T06:33:22Z |
| Matrix schema | `picogent.v4.runtime-boundary-matrix.v1` |
| Retained artifact | `/private/tmp/picogent-v4-evidence-4086b1f/runtime-boundary-matrix.json` |
| Artifact SHA-256 | `9ee2d1a56bf6fcb92cbc1e92d4944019c480c2cc4348ac7a44aa6d1a6e77441a` |

The artifact was written outside the checkout with the matrix retention
contract. Its exact-head result was:

    HEAD=PASS
    tree=CLEAN
    behavior_provenance=EXACT_HEAD
    summary: UNVERIFIED=12

The focused regression command also passed at this candidate:

    GOMAXPROCS=2 GOFLAGS=-p=1 GOTOOLCHAIN=local go test ./internal/evolve -count=1

## Claim posture

The matrix intentionally supplied no live-provider, rendered-platform, or
retained hostile-runtime artifacts. It therefore reported all twelve claims
as `UNVERIFIED` rather than projecting older observations onto this behavior
SHA.

| Claim group | Result | Evidence boundary |
| --- | --- | --- |
| hostile-child-env-sanitization; hostile-filesystem-deterministic | UNVERIFIED | Exact-head evidence documents are stale or absent for this refresh. |
| hostile-filesystem-toctou | UNVERIFIED | Broad arbitrary same-UID race residual remains explicitly unproved. |
| hostile-parent-swap-confinement | UNVERIFIED | No valid exact-head Darwin/Linux parent-swap packet was supplied. |
| live-provider-connectivity; live-provider-quality | UNVERIFIED | No exact-head provider artifacts were supplied. |
| rendered-cross-platform; rendered-long-horizon-local; rendered-platform-local; rendered-recovery-undo-reload | UNVERIFIED | No exact-head rendered artifacts were supplied. |
| restart-steer-undo-recovery | UNVERIFIED | Existing deterministic records are not silently rebound to this behavior SHA. |
| release-authorization | UNVERIFIED in this matrix | Exact-head audit evidence is stale or absent; the separate operator predicate remains unapproved. |

Historical live, rendered, hostile, and recovery packets remain separately
bound to their recorded behavior candidates. They are not unioned with this
matrix into a synthetic release-authorizing result.

## Evolution-store load-lock hardening covered by this checkpoint

PR #723 preserves first-use behavior: `Load` returns an empty workspace store
without creating `PICOGENT_HOME` state when the required parent or evolution
state directory is missing. Once an existing state directory is present, it
acquires the shared store lock before reading the payload. A non-directory
parent or state directory remains a meaningful error instead of being
mistaken for first use.

The deterministic regression holds the shared evolution lock, proves that a
concurrent `Load` cannot return while the lock is held, then confirms it reads
the existing store after release. This narrows a concrete coordination edge.

It does not make the initial parent/state existence probes, lock acquisition,
and payload read an atomic cross-process snapshot. The broad
`hostile-filesystem-toctou` claim remains `UNVERIFIED`.

## Release and parent boundary

The current formal posture remains **NOT COMPLETE**. The separate operator
release predicate remains unapproved (`authorized=false`); no release claim
is implied by this local matrix or the hosted PR checks. Parent #453 remains
open. A later documentation-only descendant may retain this exact-head packet
under the repository continuity rule, but must not silently rebind its claims
to the descendant SHA.
