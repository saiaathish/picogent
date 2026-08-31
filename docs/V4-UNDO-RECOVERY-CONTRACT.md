# Picogent v4 undo recovery contract

This contract defines the smallest durable recovery unit for undo: the latest
sealed agent turn for one task and session. It is a recovery record, not a
general history store.

## Authoritative record

The durable record is versioned, bounded to one latest turn, tied to the task
and session identity, and protected by the same workspace/path safety rules as
the existing checkpoint. It contains the pre-turn state needed to restore each
checkpointed native file, the post-seal fingerprints used for conflict
detection, and the recovery lifecycle state.

Only a validated `sealed` or `recovery-pending` record can make undo available.
Missing, malformed, unsupported-version, wrong-task/session, stale,
superseded, or otherwise unverifiable records fail closed and cannot authorize
a workspace mutation.

## Lifecycle

| State | Meaning | Undo/recovery availability |
| --- | --- | --- |
| `prepared` | The record is incomplete or has not been sealed. | Unavailable. |
| `sealed` | The turn completed its checkpoint and the record is durable. | Available for the matching task/session. |
| `recovery-pending` | Workspace mutation happened, but the task-state transition was not durably committed. | Available to a fresh process for recovery. |
| `restored` | The record was successfully consumed by undo. | Unavailable; repeated restore is rejected. |
| `committed` | The turn is durably complete and no longer undoable. | Unavailable. |
| `conflicted` | A newer workspace change prevents safe restoration. | No mutation; report the conflict and retain evidence for retry/inspection. |

The write ordering must leave either the previous valid record or the new
record recoverable. A crash between the native-file write and task-state
commit must not silently erase the latest undo candidate.

## Executable case matrix

Each implementation test must assert both the workspace result and the
authoritative user-visible outcome.

| Case | Starting workspace | Durable evidence | Expected workspace effect | Expected outcome |
| --- | --- | --- | --- | --- |
| Existing file | File exists before the turn and is changed by it. | Matching sealed record and unchanged post-seal fingerprint. | Restore original bytes and mode. | Undo succeeds and consumes the record. |
| New file | File is absent before the turn and created by it. | Matching sealed record marks the path absent. | Remove only the created file. | Undo succeeds and consumes the record. |
| Previously absent path | Path is absent before and after the turn. | Matching record. | Keep it absent. | Undo is successful/idempotent for that path. |
| Fresh process | Original process is gone. | Valid record matches task/session and latest sequence. | Same result as in-process undo. | Fresh process discovers the candidate. |
| Post-write crash | Native-file write completed before task-state commit. | `recovery-pending` record with matching write evidence. | Recover or expose the candidate without overwriting newer work. | Recovery remains retryable and observable. |
| Conflict | A checkpointed path changed after the turn. | Record is valid but post-seal fingerprint mismatches. | Leave the newer bytes/mode untouched. | Fail closed with a path-specific conflict. |
| Persistence retry | Durable state write returns a retryable error. | Record remains valid and bounded. | Do not discard the candidate. | Surface retryable failure; later retry can finish. |
| Missing/malformed | No record, truncated record, or invalid fields. | No validated evidence. | No workspace mutation. | Undo unavailable/fails closed. |
| Stale/superseded | Record is valid in isolation but not latest for task/session. | Newer sequence or incompatible identity. | No workspace mutation. | Undo unavailable/fails closed. |

## Cross-surface projection

The agent owns the availability decision and explanation. GUI, TUI, and
headless execution may format it, but must not independently infer undo from a
file listing, transcript, or cached in-memory flag. All surfaces must agree on
available, unavailable, conflict, and retryable-persistence outcomes.

Existing Safe/Fast permission behavior, local-first operation, and path/symlink
safety remain in force. This contract does not authorize shell rollback,
branch reset, automatic rollback, multi-level history, or a new user-facing
workflow.

## Evidence boundary

Focused tests can prove record serialization, process restart discovery,
workspace restoration, conflict preservation, and retryability. They do not by
themselves prove live-provider quality, rendered browser behavior,
cross-platform rendered behavior, arbitrary hostile crash windows, release
readiness, or signed supply-chain claims.
