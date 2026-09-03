# v4 lifecycle contract

Status: the scenario contract and deterministic observation helper landed in
the `internal/lifecycle` package. The 2026-09-01 entry-point checkpoint adds
local macOS fresh-process coverage for headless, TUI, and GUI interruption,
plus headless/TUI/GUI persistence-failure coverage. The conditional L
reconciliation reruns the headless, agent, GUI, and TUI process-boundary
fixtures at exact source head `f29fa074c468b96badc2f951a76f1f1a90c78b1f` and
records the result in [V4-CROSS-SURFACE-CRASH-RECOVERY.md](V4-CROSS-SURFACE-CRASH-RECOVERY.md).
The follow-up Windows console-control checkpoints add hosted evidence for the
existing headless, TUI, and GUI server-shutdown signal fixtures. Windows
rendered GUI/TUI evidence and GUI process-kill recovery remain `UNVERIFIED`
until they are observed at their actual boundaries. Arbitrary crash timing,
hostile writers, live-provider quality, and the other explicit limits in the
reconciliation record remain `UNVERIFIED`.

The existing `taskstate` model and `outcome.TurnContract` remain authoritative.
`internal/lifecycle` is a test/evidence vocabulary only: it names the matrix,
captures bounded projections, and checks that interruption or persistence
failure cannot be presented as verified completion.

## Scenario matrix

| ID | Surface | Trigger | Persisted task/turn result | Completion projection | User-visible error class | Fresh-process evidence |
| --- | --- | --- | --- | --- | --- | --- |
| `headless-eof-permission` | headless | stdin EOF during Safe permission | no turn admitted; prior durable state unchanged | no marker; not ready; fail-closed | permission | required, `UNVERIFIED` |
| `headless-signal-active-turn` | headless | SIGINT/SIGTERM during an active turn | working task; interrupted/recover turn; canceled stop | no marker; not ready; fail-closed | canceled | required, local macOS pass; hosted Windows console-control pass in [PR #377](https://github.com/saiaathish/picogent/pull/377) |
| `headless-task-save-failure` | headless | terminal durable-task save fails | last checkpoint remains working/active and resumable | no marker; not ready; fail-closed | task persistence | required; fresh-process `UNVERIFIED` |
| `tui-eof-clean-exit` | TUI | EOF/quit before a turn is admitted | existing session/task snapshot retained | no marker; not ready; fail-closed | none | not required, `UNVERIFIED` |
| `tui-signal-active-turn` | TUI | Ctrl-C/owning-context cancellation | working task; interrupted/recover turn; canceled stop | no marker; not ready; fail-closed | canceled | required, local macOS pass; hosted Windows console-control pass in [PR #379](https://github.com/saiaathish/picogent/pull/379) |
| `tui-process-kill-active-turn` | TUI | TUI process is killed during an active turn | fresh process recovers the durably admitted turn as interrupted/recover | no marker; not ready; fail-closed | none | required, local macOS pass; Windows `UNVERIFIED` |
| `tui-session-save-failure` | TUI | session save fails during completion/cleanup | durable task remains resumable; session error is visible | no marker; not ready; fail-closed | session persistence | not required, local deterministic pass |
| `gui-shutdown-active-turn` | GUI | server context shutdown during an active turn | no new admission; turn finishes durably or is interrupted before cleanup returns | no completion from shutdown alone | none | required, local macOS pass; hosted Windows console-control pass in [PR #386](https://github.com/saiaathish/picogent/pull/386) |
| `gui-process-kill-active-turn` | GUI | GUI process is killed during an active turn | fresh process recovers the durably admitted turn as interrupted/recover | no marker; not ready; fail-closed | none | required, local macOS pass; Windows `UNVERIFIED` |
| `gui-reconnect-active-turn` | GUI | SSE reconnect while a turn is active | current session/turn remains authoritative; stale transcript is not grafted | projection follows current durable task only | none | not required, local deterministic pass |
| `gui-task-save-failure` | GUI | terminal durable-task save fails | prior checkpoint remains working/active and recoverable | no marker; not ready; fail-closed | task persistence | not required, local deterministic pass |
| `gui-session-save-failure` | GUI | session save fails during reset/follow-up | current durable task is retained; session error is visible | no marker; not ready; fail-closed | session persistence | not required, local deterministic pass |

The matrix is intentionally explicit about what is not yet proven. A green
unit test or hosted package check cannot be substituted for a fresh process,
owned rendered browser, live provider, or platform-specific runtime claim.

## Checkpoint evidence

The lifecycle tests use a loopback HTTP provider and assert the durable task
store plus the shared completion projection. On macOS, the fresh-process
headless and TUI signal tests both reached the provider barrier, received
SIGINT, returned cancellation, and observed an interrupted/recovery turn in
the new process's task store. The TUI harness canonicalizes the temporary
workspace before saving config so macOS `/var` and `/private/var` spellings do
not create different project-store hashes.

The headless and TUI save-failure tests separately prove that a failed durable
or session save stays fail-closed and leaves a resumable task. These are local
deterministic integration tests, not proof of every provider or UI runtime.

The GUI lifecycle tests add the same evidence at the HTTP server boundary. A
fresh GUI process reaches a loopback provider barrier, receives SIGINT, and
leaves an interrupted/recovery turn in the task store. Reconnect adopts the
current session/task generation and rejects stale callbacks. Durable task and
session save failures emit persistence errors while retaining a fail-closed,
recoverable task. These tests exercise the GUI server and event handler
directly; they do not claim rendered-browser behavior. The exact-head GUI
process-kill fixture adds the native owner-death boundary and fresh
server-state recovery.

The GUI fresh-process shutdown fixture also reaches the provider barrier and
delivers the platform-appropriate console-control signal. Its hosted Windows
job passed through the existing GUI server `signal.NotifyContext` path and
retained the interrupted/recovery turn with a fail-closed completion projection
at the HTTP server boundary. This is recorded in [PR #386](https://github.com/saiaathish/picogent/pull/386)
from source `6fc4dadae40f52ee7193b2a7ce99ddad39a84614`, merged as
`c54501f03a44b420b61f04762461e19011c7b93e`; PR run
[33729515961](https://github.com/saiaathish/picogent/actions/runs/33729515961)
and post-merge main run
[33730015632](https://github.com/saiaathish/picogent/actions/runs/33730015632)
passed all five gates. The fixture exercises the GUI server and taskstate
seams directly; it does not claim rendered GUI behavior or Windows process-kill
recovery.

The `tui-process-kill-active-turn` fixture seeds a durable session, starts the
real TUI model against a loopback provider, waits for the persisted active
turn, and kills the child with the native process-kill boundary. A fresh
`app.LoadContext` plus `newModel` then recovers the same session as an
interrupted/recovery turn with `process_restart` metadata and a fail-closed
completion proof. This is the direct M-lane evidence in [#340](https://github.com/saiaathish/picogent/issues/340);
it does not claim rendered terminal behavior, Windows console-control
semantics, arbitrary crash windows, or live-provider quality.

The signal fixtures use a platform-appropriate child boundary. Unix retains
`cmd.Process.Signal(os.Interrupt)`; Windows creates a new process group and
delivers `CTRL_BREAK_EVENT`, which Go's signal package receives as
`os.Interrupt`. `TestHeadlessFreshProcessSignalRetainsInterruptedTurn` asserts
the canceled headless child exits with code 130 and leaves an interrupted,
recoverable turn. `TestTUIFreshProcessSignalRetainsInterruptedTurn` observes
the same interrupted/recovery contract through the real TUI model. The hosted
Windows test jobs passed these fixtures at their actual platform boundary; the
provenance is recorded below.

This resolves the Windows console-control gap for these provider-independent
headless, TUI, and GUI server-boundary fixtures only. Rendered GUI/terminal
behavior, direct process-kill recovery on Windows, arbitrary crash windows, and
live-provider quality remain `UNVERIFIED`.

The cross-surface reconciliation also reruns the existing headless signal and
agent process-kill fixtures at the same exact head. Together with the GUI and
TUI results, the focused fixtures, full suite, race suite, `go vet ./...`, and
`git diff --check` are recorded in
[V4-CROSS-SURFACE-CRASH-RECOVERY.md](V4-CROSS-SURFACE-CRASH-RECOVERY.md).

## Windows console-control provenance

The hosted Windows evidence is tied to the existing signal fixtures rather than
to production signal plumbing:

| Surface | Fixture | Source head | PR checks | Merge | Post-merge main |
| --- | --- | --- | --- | --- | --- |
| headless | `TestHeadlessFreshProcessSignalRetainsInterruptedTurn` | `3923f3251a7b93725cc485e6c60f495c60871bd2` | [33721809935](https://github.com/saiaathish/picogent/actions/runs/33721809935), all five gates PASS | `c5c5cf6d27ad275c0f4f0c00ff0817fe36eeea2c` | [33722260694](https://github.com/saiaathish/picogent/actions/runs/33722260694), all five gates PASS |
| TUI | `TestTUIFreshProcessSignalRetainsInterruptedTurn` | `6ef0d37bd63f3462fa54542ca30fe633224a2925` | [33723897413](https://github.com/saiaathish/picogent/actions/runs/33723897413), all five gates PASS | `09fe4b41de4d03dffeb1e69708ae5fd7f45ee412` | [33724323436](https://github.com/saiaathish/picogent/actions/runs/33724323436), all five gates PASS |
| GUI | `TestGUIFreshProcessShutdownRetainsInterruptedTurn` | `6fc4dadae40f52ee7193b2a7ce99ddad39a84614` | [33729515961](https://github.com/saiaathish/picogent/actions/runs/33729515961), all five gates PASS | `c54501f03a44b420b61f04762461e19011c7b93e` | [33730015632](https://github.com/saiaathish/picogent/actions/runs/33730015632), all five gates PASS |

The Windows test helper uses `CREATE_NEW_PROCESS_GROUP` and
`GenerateConsoleCtrlEvent(CTRL_BREAK_EVENT, pid)`. The record proves signal
delivery and the existing fail-closed cancellation/recovery projection at the
headless, TUI, and GUI server test seams. It is not evidence for rendered
behavior, Windows process-kill recovery, or a general Windows crash guarantee.

## Invariants

- An interrupted turn is never rendered as complete and routes to recovery.
- A task or turn save failure preserves the primary failure class alongside
  the durability failure and cannot authorize completion.
- Completion markers are accepted only through the current shared projection;
  stale markers, incomplete verification, and failed saves remain fail-closed.
- Shutdown stops new admission and cleanup is bounded; reconnect only adopts
  the current session/task snapshot.

Cross-surface crash-recovery follow-up is tracked in [#338](https://github.com/saiaathish/picogent/issues/338)
under [#311](https://github.com/saiaathish/picogent/issues/311) and parent
[#246](https://github.com/saiaathish/picogent/issues/246): the S contract row
is #339, the direct TUI fixture is #340, and the conditional L evidence
reconciliation is recorded in the cross-surface evidence document and its
issue-linked PR.
