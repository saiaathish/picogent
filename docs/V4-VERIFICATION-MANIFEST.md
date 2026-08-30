# V4 verification manifest

`cmd/verify-manifest` is a developer-facing projection of the existing
verification pipeline. It does not change task completion, goal persistence,
repair authorization, or CI orchestration.

Run it for a workspace with an exact expected commit:

```sh
go run ./cmd/verify-manifest --workspace . --expected-sha <full-commit-id>
```

The JSON artifact uses schema `picogent.verify.v1` and records:

- exact `HEAD` and whether it matches the expected full commit ID;
- Git root and clean/dirty/unknown worktree state;
- host platform and Go version;
- every targeted and broader pipeline check, status, duration, counts, and
  whether captured output was truncated;
- an explicit coverage state.

Raw command output is intentionally omitted. The manifest is bounded to
`24 KiB`; check overflow is reported as `checks_truncated`. A passing pipeline
still produces `UNVERIFIED` when exact SHA, clean provenance, complete output,
or required coverage is not proven. `UNVERIFIED` exists only in this evidence
projection and is not an existing verifier status.

## Hosted CI evidence

The `release-evidence` job in `.github/workflows/ci.yml` runs after the
cross-platform test matrix. For pull requests it checks out
`github.event.pull_request.head.sha`, rather than GitHub's synthetic merge ref,
and passes that same full commit ID to `cmd/verify-manifest`. Pushes to `main`
use the pushed commit SHA.

The job uploads `verification-manifest.json` as a bounded Actions artifact.
The artifact is review evidence for the exact tested tree; it is not, by
itself, a release approval. In particular, the manifest can remain
`UNVERIFIED` when coverage or another provenance requirement is not collected.

## Local benchmark evidence

Command:

```sh
go test ./internal/benchmark -run '^$' -bench '^BenchmarkVerificationManifest$' -benchmem -count=3
```

On 2026-08-25, Apple M3 arm64 macOS, the manifest projection measured
4.48–4.56 µs/op, 3,668 B/op, 7 allocations/op, and 1,001 output bytes/op.

These are local serialization measurements, not release-readiness or live
provider-quality claims.
