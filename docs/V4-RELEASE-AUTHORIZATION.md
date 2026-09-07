# V4 release-authorization predicate

The release-authorization predicate is a separate consumer of release
evidence. It does not make a deterministic fixture, green CI run, SBOM, or
runtime matrix row into release approval by itself.

`verify.EvaluateReleaseAuthorization` returns an explicit `PASS`, `FAIL`, or
`UNVERIFIED`/`INCONCLUSIVE` result for one expected full commit SHA. A `PASS`
requires all of the following:

- a `push` event for `refs/heads/main`;
- a clean exact-head verification manifest with complete passing checks and
  targeted coverage evidence (broader coverage is not required under the
  targeted-only coverprofile contract);
- one complete passing release-gate record for every configured required job;
- a runtime-boundary matrix bound to the same SHA, with passing live-provider,
  rendered-platform, hostile-runtime, and recovery/undo categories;
- a trusted release-attestation validation result of `PASS`; and
- an explicit operator approval bound to the same SHA and the `v4-release`
  scope.

Missing evidence is `UNVERIFIED`; an incomplete observation is
`INCONCLUSIVE`; contradictory, malformed, stale, dirty, or failed evidence is
`FAIL`. Any non-`PASS` result has `authorized: false`.

The `hostile_runtime` category is complete when the bounded hostile claims
required by the matrix are `PASS`, including
`hostile-parent-swap-confinement` when that row is present. The residual
`hostile-filesystem-toctou` row may remain `UNVERIFIED` without blocking the
predicate; it is an explicit residual audit boundary, not an unreachable
universal PASS gate.

The `live_provider` category is complete only when both its connectivity and
fixed live-provider quality rows are `PASS`. The quality row is the bounded,
self-reported `fixed-no-tool-v1` campaign documented in the runtime-boundary
matrix; its PASS does not independently attest provider identity or raw result
semantics and is not a substitute for broader provider, streaming, recovery,
or cross-platform evidence.

## Operator approval checklist

Operator approval is the remaining human gate after matrix, manifest,
attestation, and release-gate inputs are otherwise ready. Before supplying
`OperatorApproval{Approved: true, Scope: "v4-release", ...}`:

1. Confirm the candidate is an exact `main` tip SHA (`push` /
   `refs/heads/main`) with a clean worktree and matching verification
   manifest `head.match=PASS`.
2. Confirm hosted required jobs (`test`, `security`) and dependent
   `release-evidence` / `production-artifacts` succeeded at that exact SHA.
3. Confirm the verification manifest overall status is `PASS` under the
   targeted-only coverage contract (targeted coverprofile measured; broader
   `go test ./...` passed without requiring whole-repo coverage).
4. Confirm the runtime-boundary matrix is bound to the same SHA, clean, and
   every non-residual claim required by the release predicate is `PASS`.
5. Confirm trusted release attestation validation is `PASS` for the same SHA.
6. Record the approving actor identity in the trusted workflow that constructs
   the approval input; this package does not authenticate the actor.
7. Keep `#450` / `#453` open until residual runtime gaps are closed or
   explicitly accepted as residual audit boundaries. Approval is eligibility,
   not publish/deploy and not `UpdateGoal complete`.

A dry-run without operator approval must remain `UNVERIFIED` /
`authorized: false` even when every other input is `PASS`.

The operator-approval record is an input contract, not an identity system. A
trusted workflow must establish the actor's authority before supplying it.
This package only prevents the release decision from silently proceeding when
that explicit gate is absent or bound to another SHA. The predicate does not
publish, tag, deploy, or close the v4 release-readiness issues.
