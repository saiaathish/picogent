# v4 bounded hostile-runtime evidence

Status: `PASS` for the deterministic hostile-runtime boundary only. This is
the evidence record for [#480](https://github.com/saiaathishkarthik/picogent/issues/480)
under the broader observation parent [#453](https://github.com/saiaathishkarthik/picogent/issues/453).
It is not a security certification, a release authorization, or proof of every
filesystem race.

## Source identity

The deterministic test evidence was collected from the clean exact `main`
checkpoint immediately before this matrix/documentation slice:

```text
repository: github.com/saiaathishkarthik/picogent
source:     49502e89516c97c1a40436f55043a4f56b281996
runtime:    go1.26.6 darwin/arm64
host:       macOS arm64
observed:   2026-09-06 UTC
```

The source packages covered by the evidence were unchanged by the matrix
projection. The following checks were run at that exact source identity:

```text
go test ./internal/securefile ./internal/procenv ./internal/workspace -count=1
PASS

go test -race ./internal/securefile ./internal/procenv ./internal/workspace -count=1
PASS
```

## Bounded controls exercised

The package tests provide deterministic evidence for these boundaries:

- `securefile`: symlink and hard-link rejection, replacement detection before
  publication, cleanup that preserves a replaced temporary entry, atomic
  reader behavior, and Unix ancestor-swap containment for reads and writes;
- `procenv`: fresh-child removal of credential-shaped, startup, loader, pager,
  and package-manager inputs while preserving ordinary runtime values, plus
  bounded output and deadline behavior;
- `workspace`: unsafe-path and symlink/hard-link rejection, root replacement
  fail-closed capture, atomic publication, stale-content/mode conflict checks,
  and regular-file removal boundaries.

The runtime-boundary matrix projects this record as:

| Claim | Verdict | Meaning |
| --- | --- | --- |
| `hostile-filesystem-deterministic` | `PASS` | The named deterministic test families are recorded at an exact source checkpoint. |
| `hostile-filesystem-toctou` | `UNVERIFIED` | The broader same-UID race claim is deliberately not established. |

The matrix row is enabled by the presence of this exact-head evidence record.
It remains a bounded documentation projection: the record does not contain raw
logs, credentials, machine-specific secret paths, or file contents.

## Reproduction and retention

Run the package checks from a clean checkout at the source SHA recorded above.
For a later matrix checkpoint, use the exact full candidate SHA and retain the
JSON artifact outside the checkout:

```sh
go test ./internal/securefile ./internal/procenv ./internal/workspace -count=1
go test -race ./internal/securefile ./internal/procenv ./internal/workspace -count=1
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha <exact-clean-candidate-sha> \
  --out <external-artifact-path>
```

The matrix command must observe a clean worktree whose `HEAD` equals the
candidate SHA. It fail-closes on missing or malformed provenance and does not
accept an artifact path inside the workspace.

## Explicit limits

This record does not prove:

- arbitrary external-process or same-UID writer resistance between a final
  check and a later pathname operation;
- complete Windows ACL, reparse-point, or platform-specific filesystem
  behavior from this macOS observation;
- live-provider quality, rendered browser behavior, cross-platform rendered
  behavior, or recovery across every supported surface;
- a security certification, production safety guarantee, or release
  authorization.

The broader `hostile-filesystem-toctou` row remains `UNVERIFIED` until a
separate evidence record establishes that larger claim without treating these
deterministic tests as a substitute.
