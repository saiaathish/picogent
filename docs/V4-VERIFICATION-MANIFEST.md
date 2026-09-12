# V4 verification manifest

`cmd/verify-manifest` is a developer-facing projection of the existing
verification pipeline. It does not change task completion, goal persistence,
repair authorization, or CI orchestration.

## Latest exact-main refresh

The exact clean main candidate
`1ef506be13d0347d820aa0f2b857e627e928f58a` was verified on 2026-09-12 with
`GOMAXPROCS=2`, `GOFLAGS=-p=1`, and `GOTOOLCHAIN=local`. The emitted manifest
reported `HEAD=PASS`, `tree=CLEAN`, targeted `internal/verify=PASS` with
`78.85939036381514%` coverage, and broader `go test ./...=PASS` across 49
packages in `130.094415792s`.

The retained external artifacts and their digests are recorded in
[V4-RELEASE-VERIFICATION-1EF506B.md](V4-RELEASE-VERIFICATION-1EF506B.md).
This refresh corrects the older killed-at-90-seconds observation; it does not
upgrade live-provider, rendered, hostile-TOCTOU, or operator-authorization
claims.

Run it for a workspace with an exact expected commit:

```sh
go run ./cmd/verify-manifest \
  --workspace . \
  --expected-sha <full-commit-id> \
  --target internal/verify
```

`--target` is repeatable and accepts a workspace-relative Go file or
directory. Hosted release evidence targets `internal/verify` so the manifest
records a bounded, exact-head targeted check before the broader workspace
suite. Omitting `--target` intentionally records the targeted stage as
`SKIPPED`; it must not be interpreted as targeted coverage.

For hosted release-evidence coverage collection, also pass an external
coverprofile path (never inside the checkout):

```sh
go run ./cmd/verify-manifest \
  --workspace . \
  --expected-sha <full-commit-id> \
  --target internal/verify \
  --coverprofile /tmp/picogent-release-evidence/verification-coverage.out \
  --targeted-timeout 45s \
  --timeout 15m
```

Hosted `release-evidence` uses the same `15m` broader bound so full
`go test ./...` can finish. A kill under that bound remains fail-closed
`INCONCLUSIVE` (`signal: killed`); the previous `90s` bound was shorter than
typical ubuntu wall-clock for the suite and produced killed `INCONCLUSIVE`
observations even when the matrix `go test ./...` job later passed.

The JSON artifact uses schema `picogent.verify.v1` and records:

- exact `HEAD` and whether it matches the expected full commit ID;
- Git root and clean/dirty/unknown worktree state;
- host platform and Go version;
- every targeted and broader pipeline check, status, duration, counts, and
  whether captured output was truncated;
- an explicit coverage state, including a measured percent when a validated
  Go coverprofile was collected for the targeted stage.

Raw command output is intentionally omitted. The manifest is bounded to
`24 KiB`; check overflow is reported as `checks_truncated`. A passing pipeline
still produces `UNVERIFIED` when exact SHA, clean provenance, complete output,
or required targeted coverage is not proven. Targeted coverprofile collection
for `internal/verify` does not authorize whole-repository coverage or upgrade
a killed broader `go test ./...` observation to `PASS`. When the broader check
itself is `PASS`, missing broader coverage no longer keeps the overall
manifest `UNVERIFIED`. `UNVERIFIED` exists only in this evidence projection and
is not an existing verifier status.

Verification command output is bounded to `8 KiB`. If a command exits
successfully but its evidence is truncated, the verifier reports
`INCONCLUSIVE` instead of `PASS`; durable task completion and user-facing
verification status preserve that conservative result because the captured
test evidence may be incomplete.

## Hosted CI evidence

The `release-evidence` job in `.github/workflows/ci.yml` runs after the
cross-platform test matrix. For pull requests it checks out
`github.event.pull_request.head.sha`, rather than GitHub's synthetic merge ref,
and passes that same full commit ID to `cmd/verify-manifest`. Pushes to `main`
use the pushed commit SHA.

The job first emits `release-gates.json`, a bounded ledger for the required
`test` and `security` jobs, and validates it against the same candidate SHA and
event. The ledger rejects missing, duplicate, failed, mismatched, nonzero, or
truncated records; the `release-evidence` job runs with `always()` so a failed
dependency cannot silently turn into a skipped evidence job. The job uploads
`verification-manifest.json`, `release-gates.json`, and
`verification-coverage.out` as a bounded Actions artifact outside the checkout.

These artifacts are review evidence for the exact tested tree; they are not, by
themselves, a release approval. In particular, the verification manifest can
remain `INCONCLUSIVE` when the broader suite is killed, or `UNVERIFIED` when
targeted coverage or provenance is incomplete, while the required CI gate
ledger still fails closed on a missing or failed job. Broader verification is
plain `go test ./...` (no `-race`); Linux race packages and dedicated hostile
parent-swap evidence stay in separate CI steps and are not silently upgraded by
a broader PASS. For the tip-bound authorization packet and human gates, see
[V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md) and
[V4-OPERATOR-RELEASE-CHECKLIST.md](V4-OPERATOR-RELEASE-CHECKLIST.md).

## Local benchmark evidence

Command:

```sh
go test ./internal/benchmark -run '^$' -bench '^BenchmarkVerificationManifest$' -benchmem -count=3
```

On 2026-08-30 at `main` head `59be9bef64f77ffd44a234479fe2a6c7bfd308c4`,
Apple M3 arm64 macOS, the manifest projection measured 4.757–4.778 µs/op,
3,668 B/op, 7 allocations/op, and 1,001 output bytes/op across three runs.

These are local serialization measurements, not release-readiness or live
provider-quality claims.
