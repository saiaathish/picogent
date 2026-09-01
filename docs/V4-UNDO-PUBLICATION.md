# v4 undo publication boundary

Status: decision recorded on 2026-09-01 from `origin/main` at
`2c602c83d683f32f655add56784988877f6c0b6b`. This is the evidence record for
issue #274 under the v4 Outcome Engine parent #246.

## Decision

Retain the current best-effort compare-then-publish boundary. Do not add a
platform-specific replacement primitive for arbitrary same-UID writers that
do not participate in Picogent's project lock.

The current boundary is the strongest behavior that can be provided with one
consistent implementation on the supported Unix/macOS and Windows targets:

1. `UndoLastTurn` acquires the project run lock before it loads or restores a
   checkpoint (`internal/agent/undo.go`). This serializes supported Picogent
   runs.
2. Existing files are preflight-checked for bounded content and mode equality
   through `WriteAtomicIfUnchangedWithMode`; files deleted by the turn use
   `WriteAtomicIfMissingWithMode`; newly created files use
   `RemoveIfUnchanged` (`internal/workspace/workspace.go`).
3. The replacement is fully written and synced to a temporary file, the
   temporary inode/handle is checked, and the final publication uses a
   descriptor-anchored Unix rename or a handle-based Windows rename
   (`internal/workspace/workspace_unix.go`,
   `internal/workspace/workspace_atomic_windows.go`).
4. A pre-publication recovery hook can persist the undo journal. A checked
   mismatch returns `ErrContentConflict` and leaves the observed newer file in
   place; the agent does not claim that undo preserved a change it could not
   verify.

These checks protect ordinary readers from partial files, reject symlink,
reparse-point, and unsafe hard-link targets, and detect a writer observed
before publication. They do not make a check followed by a pathname
replacement into a cross-process compare-and-swap.

## Platform research

| Target and primitive | Strongest relevant property | Missing property for undo CAS |
| --- | --- | --- |
| POSIX `rename()` / `renameat()` on Unix | Replaces a destination name with a complete source file, with pathname-relative directory anchoring available through `renameat()`. | No operation accepts an expected destination inode, file identity, content digest, or mode and then conditionally replaces that destination. |
| Linux `renameat2(RENAME_NOREPLACE)` | Fails with `EEXIST` when the destination already exists, which is useful for an absent-target create race. | It cannot conditionally replace an existing target. It is Linux-specific and filesystem support varies, so it cannot define the shared restore contract. |
| macOS `renameatx_np(..., RENAME_EXCL)` | Provides an exclusive no-replace form for the destination-name case. | It is still a no-replace check, not an expected-identity conditional replacement for an existing file. |
| Windows `ReplaceFileW` and handle-based `FileRenameInfo`/NT rename | Provides an OS-level replacement path; the source can be held by a handle and the current implementation uses a handle-based rename with replace semantics. | The operation has no expected destination file-ID/content/mode precondition that closes a replacement race after the last check. |

The platform references are:

- [POSIX `rename()` specification](https://pubs.opengroup.org/onlinepubs/9699919799/functions/rename.html)
- [Linux `rename(2)` and `renameat2()`](https://man7.org/linux/man-pages/man2/rename.2.html)
- [Apple `rename(2)` man page](https://developer.apple.com/library/archive/documentation/System/Conceptual/ManPages_iPhoneOS/man2/rename.2.html)
- [Apple XNU `renameatx_np` flags](https://raw.githubusercontent.com/apple-oss-distributions/xnu/main/bsd/sys/stdio.h)
- [Windows `ReplaceFileW`](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-replacefilea)
- [Windows `SetFileInformationByHandle`](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-setfileinformationbyhandle)

## Why no platform-specific guard was added

Linux's no-replace flag and macOS's exclusive flag solve only the missing
destination case. An undo of an existing file must replace a destination, so
neither flag supplies the required conditional operation. Windows replacement
and rename APIs likewise select a destination by name while offering no
expected-target identity parameter. Reading an identity first and then using
any of these operations leaves the same post-check race.

An advisory lock or lease would help only if every writer honored it. The
existing project run lock is therefore retained for cooperative Picogent
processes, while an arbitrary same-UID editor remains outside the guarantee.
Adding divergent platform branches would increase maintenance and produce no
stronger cross-platform contract.

## Deterministic evidence

The current source and test boundary is covered by:

- `TestWriteAtomicIfUnchangedRefusesStaleContent`
- `TestWriteAtomicIfUnchangedWithModeRefusesStaleMode`
- `TestWriteAtomicPublishHookRunsBeforeRename`
- `TestWriteAtomicPublishHookCanAbortPublication`
- `TestRejectedPublishDoesNotCreateUndo`
- `TestEditContentConflictDoesNotCreateUndo`
- `TestMixedWriteAndContentConflictDoesNotUndoNewerPath`
- `TestFreshUndoConflictPreservesNewerWorkspaceEdit`
- `TestUndoWaitsForActiveRunToReleaseProjectLock`

The focused Unix/macOS validation for this record is:

```sh
go test ./internal/workspace ./internal/agent -count=1
go test -race ./internal/workspace ./internal/agent -count=1
```

These checks prove the supported cooperative and pre-publication conflict
behavior. They do not prove protection from an uncooperative external writer
that wins the final pathname race, a multi-file transactional restore, live
provider behavior, rendered UI behavior, or release readiness. The hosted
cross-platform CI result for the implementation PR must be recorded on issue
#274; it does not turn the residual race into a solved guarantee.

## Future scope boundary

A stronger guarantee would require a separately designed cooperative writer
protocol, a supported filesystem-specific conditional primitive, or a
different restore model that does not publish through an unprotected final
pathname. Any of those would need an explicit product and cross-platform
compatibility decision rather than an inferred security promise.
