# v4 lifecycle contract

Status: the scenario contract and deterministic observation helper landed in
the `internal/lifecycle` package. The 2026-09-01 entry-point checkpoint adds
local macOS fresh-process coverage for headless, TUI, and GUI interruption,
plus headless/TUI/GUI persistence-failure coverage. Hosted Windows signal
behavior and all rendered GUI/TUI evidence remain `UNVERIFIED` until they are
observed at their actual boundaries. The contract now also names abrupt TUI
process-kill recovery; its direct fixture is tracked separately and remains
`UNVERIFIED` until that evidence lands.

The existing `taskstate` model and `outcome.TurnContract` remain authoritative.
`internal/lifecycle` is a test/evidence vocabulary only: it names the matrix,
captures bounded projections, and checks that interruption or persistence
failure cannot be presented as verified completion.

## Scenario matrix

| ID | Surface | Trigger | Persisted task/turn result | Completion projection | User-visible error class | Fresh-process evidence |
| --- | --- | --- | --- | --- | --- | --- |
| `headless-eof-permission` | headless | stdin EOF during Safe permission | no turn admitted; prior durable state unchanged | no marker; not ready; fail-closed | permission | required, `UNVERIFIED` |
| `headless-signal-active-turn` | headless | SIGINT/SIGTERM during an active turn | working task; interrupted/recover turn; canceled stop | no marker; not ready; fail-closed | canceled | required, local macOS pass; Windows `UNVERIFIED` |
| `headless-task-save-failure` | headless | terminal durable-task save fails | last checkpoint remains working/active and resumable | no marker; not ready; fail-closed | task persistence | required; fresh-process `UNVERIFIED` |
| `tui-eof-clean-exit` | TUI | EOF/quit before a turn is admitted | existing session/task snapshot retained | no marker; not ready; fail-closed | none | not required, `UNVERIFIED` |
| `tui-signal-active-turn` | TUI | Ctrl-C/owning-context cancellation | working task; interrupted/recover turn; canceled stop | no marker; not ready; fail-closed | canceled | required, local macOS pass; Windows `UNVERIFIED` |
| `tui-process-kill-active-turn` | TUI | TUI process is killed during an active turn | fresh process recovers the durably admitted turn as interrupted/recover | no marker; not ready; fail-closed | none | required, `UNVERIFIED` pending direct fixture |
| `tui-session-save-failure` | TUI | session save fails during completion/cleanup | durable task remains resumable; session error is visible | no marker; not ready; fail-closed | session persistence | not required, local deterministic pass |
| `gui-shutdown-active-turn` | GUI | server context shutdown during an active turn | no new admission; turn finishes durably or is interrupted before cleanup returns | no completion from shutdown alone | none | required, local macOS pass; Windows `UNVERIFIED` |
| `gui-process-kill-active-turn` | GUI | GUI process is killed during an active turn | fresh process recovers the durably admitted turn as interrupted/recover | no marker; not ready; fail-closed | none | required, `UNVERIFIED` |
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
directly; they do not claim rendered-browser behavior.

The `tui-process-kill-active-turn` row is a contract requirement only at this
checkpoint. The existing TUI signal test does not substitute for an abrupt
owner-death boundary; the direct child-kill and fresh-model observation is
tracked by [#338](https://github.com/saiaathish/picogent/issues/338).

Windows hosted SIGINT behavior is intentionally not claimed here: the current
child-process boundary does not provide a stable `os.Interrupt` delivery
contract on Windows. The Windows lifecycle row stays `UNVERIFIED` until a
platform-appropriate console-control test or direct runtime observation is
recorded.

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
is #339, the direct TUI fixture is #340, and the conditional evidence
reconciliation remains a later L checkpoint.
