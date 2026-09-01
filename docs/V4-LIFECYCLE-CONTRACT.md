# v4 lifecycle contract

Status: the scenario contract and deterministic observation helper landed in
the `internal/lifecycle` package. The 2026-09-01 entry-point checkpoint adds
local macOS fresh-process coverage for headless and TUI interruption, plus
headless/TUI persistence-failure coverage. Hosted Windows signal behavior and
all rendered GUI/TUI evidence remain `UNVERIFIED` until they are observed at
their actual boundaries.

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
| `tui-session-save-failure` | TUI | session save fails during completion/cleanup | durable task remains resumable; session error is visible | no marker; not ready; fail-closed | session persistence | not required, local deterministic pass |
| `gui-shutdown-active-turn` | GUI | server context shutdown during an active turn | no new admission; turn finishes durably or is interrupted before cleanup returns | no completion from shutdown alone | none | required, `UNVERIFIED` |
| `gui-reconnect-active-turn` | GUI | SSE reconnect while a turn is active | current session/turn remains authoritative; stale transcript is not grafted | projection follows current durable task only | none | not required, `UNVERIFIED` |
| `gui-task-save-failure` | GUI | terminal durable-task save fails | prior checkpoint remains working/active and recoverable | no marker; not ready; fail-closed | task persistence | not required, `UNVERIFIED` |
| `gui-session-save-failure` | GUI | session save fails during reset/follow-up | current durable task is retained; session error is visible | no marker; not ready; fail-closed | session persistence | not required, `UNVERIFIED` |

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

Follow-up work is tracked in [#281](https://github.com/saiaathish/picogent/issues/281)
under parent [#246](https://github.com/saiaathish/picogent/issues/246):
headless/TUI coverage first, then GUI shutdown/reconnect/save-failure evidence.
