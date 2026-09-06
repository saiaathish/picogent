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

## Retained-artifact publication checkpoint

PR #483 extends this bounded record at source checkpoint
`a76802e3d39b56290d8652257b4e4accfb82ecaf`. `runtimeboundary.RetainReport`
now delegates exclusive artifact creation to `securefile.WriteExclusive`,
which creates the parent through the descriptor/handle-anchored implementation,
refuses overwrite, and removes only the inode it opened when a write fails.

The following focused checks passed at that exact source identity:

```text
go test ./internal/securefile ./internal/runtimeboundary -count=1
PASS

go test -race ./internal/securefile ./internal/runtimeboundary -count=1
PASS
```

The Unix-only `TestRetainReportParentSwapNeverEscapesDescriptor` campaign
repeatedly renames the intended parent away, presents a symlink to a separate
outside directory, and restores the parent while retained artifacts are
attempted. The outside directory retained only its sentinel file. Failures
during the hostile interval are accepted; a successful operation must not
escape into the hostile target.

This is a bounded Unix observation of retained-artifact parent replacement. It
does not establish Windows reparse-point behavior, arbitrary same-UID races
after every final identity check, or a cross-surface guarantee. Therefore the
matrix row `hostile-filesystem-toctou` remains `UNVERIFIED`.

## Target identity checkpoint

PR #485 adds a second bounded publication check at source checkpoint
`70397264f5cd0ceeffe7dbe4a8b31870f4b6ee05`. `securefile.WriteExclusive` now
compares the opened descriptor/handle with the current target name after the
payload is synced. If a same-UID writer has renamed the trusted entry and
installed a replacement, the operation fails before reporting success and
inode-aware cleanup leaves the replacement untouched.

The Unix `TestWriteExclusiveRejectsReplacedTarget` test exercises that exact
sequence and verifies both the attacker replacement and the trusted renamed
entry. Windows and fallback implementations compile against the same identity
contract and remain covered by hosted platform checks, but this record is not a
Windows hostile-writer observation.

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

## macOS same-UID parent-swap harness checkpoint

Issue [#496](https://github.com/saiaathish/picogent/issues/496) adds a
Darwin-only, separate-process hostile parent-swap harness for
`securefile` and `workspace` confinement operations. The digest-only evidence
contract and observed checkpoint are recorded in
[V4-HOSTILE-PARENT-SWAP-DARWIN.md](V4-HOSTILE-PARENT-SWAP-DARWIN.md).

That harness can `PASS` when confirmed attacker activity never mutates the
outside sentinel or any other entry in the outside tree, and successful
operations stay descriptor-anchored. The current retained Darwin artifacts
were collected at exact source `effe52a21f770bc435bbb1596df1452339cbd82a`:

- securefile artifact: `1823593e9d9de2ed5bfec45ccd519da005cdaab550e197f8b38bf91bfdc56273`;
- workspace artifact: `8dfe6a54f26bc664e5972e85ebb0180a896111ebeb8b33953cfd9b44c6da03ce`.

They include same-name outside read markers, complete outside-tree digests,
operation counts, attacker activity, and Darwin/arm64 provenance. It still
does **not** upgrade `hostile-filesystem-toctou`.

The exercised attacker presents a symlink after renaming the trusted parent.
Replacement with an ordinary attacker-owned directory between permission
approval and file-tool execution is intentionally outside this slice and
remains an unresolved P1 boundary.
