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
| `hostile-parent-swap-confinement` | `PASS` when both Darwin and Linux parent-swap evidence docs are present | Bounded same-UID parent-swap confinement only. |
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

The residual `hostile-filesystem-toctou` row remains `UNVERIFIED`. Bounded
parent-swap confinement is claimed separately by
`hostile-parent-swap-confinement` when both Darwin and Linux evidence records
are present; that narrowed PASS does not substitute for the broader residual.

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
operation counts, attacker activity, and Darwin/arm64 provenance. Together with
the Linux parent-swap evidence record, they enable
`hostile-parent-swap-confinement=PASS`. They still do **not** upgrade
`hostile-filesystem-toctou`.

The exercised attacker presents a symlink after renaming the trusted parent.
Replacement with an ordinary attacker-owned directory between permission
approval and file-tool execution is covered by the separate workspace-root
identity binding work under [#502](https://github.com/saiaathish/picogent/issues/502).
Broad arbitrary same-UID TOCTOU across every surface remains `UNVERIFIED`.

Issue [#517](https://github.com/saiaathish/picogent/issues/517) extends the
Darwin parent-swap harness to sealed checkpoint `Restore`. The digest-only
contract and commands live in
[V4-HOSTILE-PARENT-SWAP-DARWIN.md](V4-HOSTILE-PARENT-SWAP-DARWIN.md#checkpoint-restore-campaign).
Local Darwin validation at pre-merge source recorded `PASS` with unchanged
outside sentinel/tree digests and confirmed attacker swaps for both restore
write and restore-delete operations. The checkpoint artifact SHA256 is
`14ab897e1ddd528c5118baed7346034bb455cecb7f7e7dfed11a6a23c55806a7`.
`hostile-filesystem-toctou` stays `UNVERIFIED`.

## Linux same-UID parent-swap harness checkpoint

Issue [#504](https://github.com/saiaathish/picogent/issues/504) extends the
same separate-process parent-swap confinement harness to Linux for
`securefile` and `workspace`. The digest-only evidence contract and commands
are recorded in
[V4-HOSTILE-PARENT-SWAP-LINUX.md](V4-HOSTILE-PARENT-SWAP-LINUX.md).

The hosted Ubuntu workflow retained artifacts at source
`bace7ecbf4da0118c543066beacdbff6a058f857` from
[CI run 34037551757](https://github.com/saiaathish/picogent/actions/runs/34037551757).
The Linux/amd64 securefile artifact is
`ce769fd8eb2a316de9e6f1946b30e01c31df1cdd19aa03beb69f153f6b73fa61` and the
workspace artifact is
`c1b4758ebbd97f27e18dc034e4a48345a6bdc7b31bb169b86ac868f04d675756`. Both
reported `PASS` with confirmed attacker activity and unchanged outside
sentinel/tree digests. The bounded harness required successful in-tree
operations, rejected dirty or stale source identity, cleaned up attacker
helpers, and hashed nested outside-tree entries without following symlinks.

Issue [#523](https://github.com/saiaathish/picogent/issues/523) /
PR [#526](https://github.com/saiaathish/picogent/pull/526) extends the same
Linux harness to sealed checkpoint `Restore`
(`TestLinuxSameUIDCheckpointParentSwapConfinement`), mirroring Darwin [#519](https://github.com/saiaathish/picogent/pull/519).
Post-merge Ubuntu CI on `5d4496b88a8ed2d86d1bee8705eff86c14763483` retained
`checkpoint.json` with overall `PASS` and
`artifact-sha256=7ee2ee5f6c7da728a7a17f0f6831884de2495284471ca63a1a6dfa89935a9914`.
Digest-only evidence keeps `BroadTOCTOUClaim: UNVERIFIED`.

Issue [#566](https://github.com/saiaathishkarthik/picogent/issues/566) /
PR [#567](https://github.com/saiaathishkarthik/picogent/pull/567) adds the
separate Linux same-UID final-path replacement campaign for the four named
`securefile` operations. Its exact-head digest-only result and explicit limits
are recorded in
[V4-HOSTILE-FINAL-PATH-LINUX.md](V4-HOSTILE-FINAL-PATH-LINUX.md). The hosted
candidate `c44d8825732877aac9b33ca7bfd448a18085918f` reported `PASS` with
confirmed attacker swaps and unchanged outside-tree digests. This is a
narrowed final-path observation only; `hostile-filesystem-toctou` remains
`UNVERIFIED`.

The row `hostile-filesystem-toctou` stays `UNVERIFIED` until a broader
evidence record exists.

## Retained-artifact read-side campaign (#624)

Issue [#624](https://github.com/saiaathishkarthik/picogent/issues/624) adds a
bounded read-side observation for `runtimeboundary.LoadReport`. An independent
same-UID helper repeatedly renames the trusted artifact parent away, presents a
symlink to a separate outside directory containing a valid candidate-matching
artifact with an outside-only marker, and restores the trusted parent. The test
passes only when the outside marker is never accepted, at least one in-tree load
succeeds, the helper confirms activity, and the outside artifact digest is
unchanged.

The first exact clean local observation used behavior source
`2c625c88149b4dbebadba106ecf58908683c8023` on Darwin/arm64:

```text
schema=picogent.v4.hostile-retained-artifact-read-evidence.v1
attempts=400
successful_loads=54
confirmed_attacker_swaps=true
outside_marker_observed=false
source_tree_modified=false
verdict=PASS
broad_toctou_claim=UNVERIFIED
artifact_sha256=e02dcec92a5ece246264a80e55b280efea91640a9c10210a1a90b598aa3b8746
artifact=/private/tmp/picogent-hostile-retain-624.zDHqYp/retained-artifact-read.json
```

After the pacing fix, a fresh exact clean local observation used behavior
source `e7f90b2fbc15b1a5e190a27c1884a365f3dcdd86` on Darwin/arm64:

```text
schema=picogent.v4.hostile-retained-artifact-read-evidence.v1
attempts=400
successful_loads=122
confirmed_attacker_swaps=true
outside_marker_observed=false
source_tree_modified=false
verdict=PASS
broad_toctou_claim=UNVERIFIED
artifact_sha256=e7872eebda5f9d910708c6926b8a500567b6d6d9011467a736f29ec6889a1939
```

PR #625's hosted run for this fixed source passed Ubuntu, Windows, macOS,
security, production-artifacts, and release-evidence checks.

The Unix CI matrix runs the same opt-in evidence mode on the exact pull-request
source SHA and uploads only the digest-only JSON artifact. Normal test runs do
not retain files or start the helper outside this focused test. This is a
bounded retained-artifact read-confinement result; it does not establish
Windows reparse behavior, every pathname operation, or the broad
`hostile-filesystem-toctou` claim. That matrix row remains `UNVERIFIED`.

## Windows retained-artifact read campaign (#626)

Issue [#626](https://github.com/saiaathishkarthik/picogent/issues/626) adds the
platform-specific counterpart for the retained-artifact read boundary. The
focused Windows test starts an independent same-UID helper that replaces the
trusted parent name with a junction to an outside directory during repeated
`LoadReport` calls. The outside artifact carries a valid candidate-matching
record with an `outside-marker` reason; a successful load must never return
that marker, at least one trusted in-tree load must succeed, and the outside
artifact digest must remain unchanged.

The implementation and CI contract are documented in
[V4-HOSTILE-RETAINED-READ-WINDOWS.md](V4-HOSTILE-RETAINED-READ-WINDOWS.md).
The hosted observation is retained only in the runner's temporary artifact
directory and must be rebound here with its exact source SHA and digest after a
passing Windows run. Until that record is available, this section is an
implemented contract, not a Windows `PASS` claim. The broad
`hostile-filesystem-toctou` row remains `UNVERIFIED`.

## Project-rule read campaign (#628)

Issue [#628](https://github.com/saiaathishkarthik/picogent/issues/628) closes a
separate source-backed read boundary: the project instruction loader no longer
uses an unrestricted path read. `AGENTS.md`, `CLAUDE.md`, and
`.picogent/rules.md` are read through the shared descriptor/handle-anchored
securefile boundary and retain the existing 24 KiB prefix limit.

The focused Unix and Windows tests start an independent same-UID helper that
replaces `.picogent` with a symlink or junction during 256 `Load` calls. The
outside directory contains an `outside-project-rule-marker`; a passing
campaign must never return it, must observe at least one trusted in-tree load,
and must leave the outside file digest unchanged. The exact protocol and
digest-only schema are documented in
[V4-PROJECT-RULE-READ-EVIDENCE.md](V4-PROJECT-RULE-READ-EVIDENCE.md).

Hosted observations are retained only in runner temporary artifacts and must
be rebound here with the exact source SHA and digest after the CI run passes.
Until then this is an implemented contract, not a hosted `PASS` claim. The
broad `hostile-filesystem-toctou` row remains `UNVERIFIED`.
