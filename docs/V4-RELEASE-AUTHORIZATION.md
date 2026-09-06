# V4 release-authorization predicate

The release-authorization predicate is a separate consumer of release
evidence. It does not make a deterministic fixture, green CI run, SBOM, or
runtime matrix row into release approval by itself.

`verify.EvaluateReleaseAuthorization` returns an explicit `PASS`, `FAIL`, or
`UNVERIFIED`/`INCONCLUSIVE` result for one expected full commit SHA. A `PASS`
requires all of the following:

- a `push` event for `refs/heads/main`;
- a clean exact-head verification manifest with complete passing checks and
  coverage evidence;
- one complete passing release-gate record for every configured required job;
- a runtime-boundary matrix bound to the same SHA, with passing live-provider,
  rendered-platform, hostile-runtime, and recovery/undo categories;
- a trusted release-attestation validation result of `PASS`; and
- an explicit operator approval bound to the same SHA and the `v4-release`
  scope.

Missing evidence is `UNVERIFIED`; an incomplete observation is
`INCONCLUSIVE`; contradictory, malformed, stale, dirty, or failed evidence is
`FAIL`. Any non-`PASS` result has `authorized: false`.

The `live_provider` category is complete only when both its connectivity and
fixed live-provider quality rows are `PASS`. The quality row is the bounded
`fixed-no-tool-v1` campaign documented in the runtime-boundary matrix; it is
not a substitute for broader provider, streaming, recovery, or cross-platform
evidence.

The operator-approval record is an input contract, not an identity system. A
trusted workflow must establish the actor's authority before supplying it.
This package only prevents the release decision from silently proceeding when
that explicit gate is absent or bound to another SHA. The predicate does not
publish, tag, deploy, or close the v4 release-readiness issues.
