# V4 hostile filesystem TOCTOU boundary inventory

Status: **inventory and contract slice only**. This record supports the
remaining [#453](https://github.com/saiaathish/picogent/issues/453) audit and
records the completed #648 artifact-boundary child. It does not claim
`hostile-filesystem-toctou=PASS`, authorize a release, or provide race-winning
or exploit procedures.

## Evidence anchor

The inventory was reviewed against merged `main` at
`9c1ca2e4df5af35dde5ed5e1f5cce39368e9b2fb` after focused hardening in PRs
#657, #659, and #661. The latest rendered evidence
candidate remains
`592b07a633d9683354c4abeee09b767bc071ab35`; its exact-candidate aggregate and
runtime matrix are documented in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-592B07A.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-592B07A.md).
That packet records `rendered-cross-platform=PASS`, while the broader
`hostile-filesystem-toctou` row remains `UNVERIFIED`.

## Boundary inventory

| Surface | Current owner and mechanism | Bounded evidence | Remaining boundary |
| --- | --- | --- | --- |
| Permission-to-file approval | `internal/perm/perm.go`: `ClassifyPath`, `ResolveWorkspacePath`, `WorkspaceIdentity.Validate` capture a canonical root identity and require revalidation before path-scoped I/O. | Root replacement is rejected at the approval boundary; path aliases are resolved fail-closed. | The approval check and the later filesystem operation are separate events. Hostile races after the final validation are delegated to the platform-specific workspace primitives. |
| Workspace file access | `internal/workspace/workspace.go` plus `workspace_unix.go` / `workspace_windows.go` traverse from a root descriptor/handle, reject symlinks/reparse points and hard-linked files, and publish atomic replacements. | `workspace/hostile_parent_swap_*` and workspace security tests cover bounded parent replacement and safe failure. | `WriteAtomicIfUnchanged*` is compare-then-publish, not a cross-process CAS. Final-name replacement and replacement of the named root remain outside a universal same-UID guarantee. |
| Shared secure-file primitive | `internal/securefile/securefile.go` plus Unix/Windows implementations open the parent by descriptor/handle, use no-follow checks, compare identities for cleanup, and publish through the anchored parent. | `securefile/hostile_parent_swap_*` and final-path tests cover the named bounded families. | The implementation documents that a same-UID writer can race a final pathname or rename after an identity check. That is a narrower race detector/confinement contract, not universal TOCTOU closure. |
| Checkpoint and undo restore | `internal/checkpoint/checkpoint.go`: `resolveWorkspace` / `securePath` preflight paths; `readWorkspaceFile` and `writeWorkspaceState` delegate native mutations to `workspace`; `Restore` handles conflicts and rollback. `internal/agent/undo_journal.go` persists journals through `securefile`. | Recovery/undo tests cover conflict detection, restart persistence, and bounded parent-swap behavior. | `EvalSymlinks`/`Lstat` preflight and later operations are separate observations. A replaced root, unlisted path family, or uncooperative writer can still produce a safe failure or a conflict without proving a universal no-race claim. |
| Session, task, goal, and trace state | Active session/delete-undo, task-state, goal, and trace payload reads/writes use `securefile` plus application locks. Goal primary/backup existence probes now use bounded securefile reads. Directory enumeration and multi-record workflows still use ordinary path operations at their coordination edges. The exact-head call-site inventory found no callers for the legacy raw `internal/session/atomic*.go` helpers; those dead helpers were removed. | Bounded file limits, atomic publication, lock ownership, restart recovery, state validation, and Unix symlink rejection for goal probes are covered by their package tests. | The secure-file payload contract does not make directory enumeration or multi-file transactions an atomic cross-process snapshot. Session directory coordination and multi-record workflows remain separate residual surfaces. |
| Evolution store state | `internal/evolve/store.go` now uses `securefile.EnsureDir`, `ReadFile`, and `WriteAtomic`; the shared `securefile.OpenLockFile` / `LockFile` contract owns lock bootstrap and platform locking. The old raw `internal/evolve/atomic*.go` and platform lock helpers were removed. | Evolution-store tests cover ordinary persistence, cross-process serialization, and Unix rejection of symlinked state parents, state files, and lock files; securefile platform tests cover the underlying descriptor/handle implementations. | The secure-file contract narrows pathname replacement and symlink races but does not provide an atomic cross-process snapshot across the initial existence probe, lock acquisition, and payload read. The broad same-UID TOCTOU claim remains `UNVERIFIED`. |
| Retained runtime evidence | `internal/runtimeboundary/retain.go`: lexical/resolved outside-workspace validation then `securefile.WriteExclusive`; loads use `securefile.ReadFileLimited`. | Exact-SHA schema, size, candidate, clean-tree, and outside-workspace checks are covered; the current rendered packet retains digest-only artifacts. | The resolved artifact parent is not a universal immutable identity binding. A hostile writer can still race an outside path after validation; the matrix intentionally keeps the broad row `UNVERIFIED`. |
| GUI workspace preview | `internal/gui/server.go` `readFile` resolves the request with `perm` and opens through `workspace.OpenRead`, then reads a bounded prefix. | Read-only preview is bounded and uses the workspace file boundary for the actual open. | This is a read-only consumer, not proof for every other path surface. The separate file-picker attachment path in `internal/gui/files_api.go` reads explicitly user-selected paths with `os.ReadFile` and has a different trust boundary. |
| Release-artifact output | `internal/releaseartifact/artifact.go` stages binaries, packages, and SBOMs in an owned temporary directory; staged/final reads use `securefile`, final publication uses `securefile.WriteAtomic`, and the package archive is written only inside the owned stage. | Hosted release-artifact checks validate clean provenance, deterministic outputs, and artifact contracts in task-owned CI environments. | The stage archive writer and final output names still use path operations within their respective owned directories. A hostile same-UID writer can race a final name after identity checks; this is outside the bounded contract. |
| Matrix/manifest consumers | `cmd/runtime-boundary-matrix`, `cmd/release-gates`, and `cmd/verify-manifest` use absolute-path and bounded-file validation before consuming retained artifacts or ledgers. Release-gate ledgers and targeted Go coverprofiles now read through `securefile.ReadFileLimited`. | Malformed, stale-SHA, trailing, oversized, symlinked, and workspace-contained artifacts fail closed where the contracts apply; Unix tests cover symlinked ledger/profile targets and parents. | Consumer validation is not an atomic read/verify transaction against an uncooperative same-UID writer. It proves artifact contract handling, not universal filesystem race resistance. |

## Decision boundary

The inventory separates three outcomes that must not be conflated:

1. **Bounded hardening:** a concrete product surface has a meaningful
   descriptor/handle-anchored fix, focused platform tests, and a narrowly stated
   claim. This can be implemented in its own small PR.
2. **Bounded evidence:** a hostile campaign proves only the named operation and
   platform family. Its result can update that claim, but never the universal
   TOCTOU row by implication.
3. **Residual acceptance:** the universal claim cannot be proved without
   widening into an unsafe or unbounded attack campaign. In that case retain
   `UNVERIFIED`, document covered and excluded surfaces, and obtain a separate
   human operator decision through the residual-acceptance record.

No implementation should be started solely to change the matrix count. The
#648 artifact-output decision has now produced focused hardening slices; the
remaining #453 work should use the same bar. Otherwise the correct deliverable
is a more precise residual boundary and operator packet.

## Proposed validation contract

Any follow-up implementation or evidence PR should bind:

- exact source SHA and a clean tree;
- one owned disposable environment per platform claim;
- bounded artifacts containing identities and digests, not credentials or raw
  transcripts;
- explicit `PASS`, `FAIL`, `INCONCLUSIVE`, or `UNVERIFIED` verdicts;
- a focused test command with low parallelism on constrained developer hosts;
- post-merge verification that does not project evidence onto a later
  non-documentation candidate.

The parent issue remains open until the residual is either independently
resolved or consciously accepted and the separate human release decision is
recorded.
