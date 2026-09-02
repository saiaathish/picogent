# v4 cross-surface crash-recovery evidence

Status: the conditional L lane in [#338](https://github.com/saiaathishkarthik/picogent/issues/338) has direct local macOS evidence at the exact merged source head `f29fa074c468b96badc2f951a76f1f1a90c78b1f`. The evidence covers the existing headless and agent process boundaries plus the GUI and TUI process-kill fixtures. It is a bounded recovery record, not a release-readiness claim.

## Scope and source identity

The evidence was rerun on 2026-09-02 from the clean branch
`codex/v4-crash-recovery-evidence` at:

```text
source/base: f29fa074c468b96badc2f951a76f1f1a90c78b1f
runtime:     go1.26.6 darwin/arm64
repository:  github.com/saiaathish/picogent
```

Each fixture uses a disposable home/workspace and a deterministic loopback
provider or an existing process-harness barrier. The assertions inspect the
durable task/session state and the shared completion projection after the
owner process is interrupted or killed.

## Cross-surface result

| Surface | Fixture | Boundary exercised | Result | Direct assertion |
| --- | --- | --- | --- | --- |
| Headless | `TestHeadlessFreshProcessSignalRetainsInterruptedTurn` | Fresh headless child reaches the provider barrier, receives `SIGINT`, and exits with cancellation | PASS, local macOS | The durable task remains retained as interrupted/recover and the completion projection stays fail-closed |
| Agent | `TestLongHorizonResumeAfterProcessKill` | A worker holding the active-turn/run-lock boundary is killed, then a fresh process resumes the same session | PASS, local macOS | The interrupted `process_restart` turn is retained, remains unchanged before follow-up, and the next turn completes without a completion claim |
| GUI | `TestGUIFreshProcessKillRecoversInterruptedTurn` | Real GUI server child reaches an active loopback request and is terminated with `Process.Kill` | PASS, local macOS | Fresh recovery loads the same session as a working interrupted/recover turn with `process_restart` metadata and a not-ready completion projection |
| TUI | `TestTUIFreshProcessKillRecoversInterruptedTurn` | Real TUI model child reaches a durably persisted active turn and is terminated with `Process.Kill` | PASS, local macOS | Fresh `app.LoadContext` plus `newModel` recovers the same session as an interrupted/recover turn with `process_restart` metadata and a not-ready completion projection |

The headless fixture proves the existing signal boundary. The agent fixture
proves the existing direct-kill/resume path. GUI and TUI use their real server
and model/session seams; neither fixture adds a second lock, store, planner,
daemon, watcher, renderer, or user-facing workflow.

## Validation record

Focused fixtures, each run once at the source head above:

```text
go test ./cmd/picogent -run '^TestHeadlessFreshProcessSignalRetainsInterruptedTurn$' -count=1 -timeout=180s
ok   github.com/saiaathish/picogent/cmd/picogent  2.783s

go test ./internal/agent -run '^TestLongHorizonResumeAfterProcessKill$' -count=1 -timeout=180s
ok   github.com/saiaathish/picogent/internal/agent  1.064s

go test ./internal/gui -run '^TestGUIFreshProcessKillRecoversInterruptedTurn$' -count=1 -timeout=180s
ok   github.com/saiaathish/picogent/internal/gui  2.350s

go test ./internal/tui -run '^TestTUIFreshProcessKillRecoversInterruptedTurn$' -count=1 -timeout=180s
ok   github.com/saiaathish/picogent/internal/tui  1.848s
```

Exact-head repository gates also passed:

```text
go test ./... -count=1 -timeout=600s                         PASS
go test -race ./cmd/picogent ./internal/agent ./internal/gui ./internal/tui ./internal/taskstate -count=1 -timeout=600s  PASS
go vet ./...                                                  PASS
git diff --check                                             PASS
```

The full-suite run included `internal/measure`, whose isolated fixed-benchmark
fixture passed separately in 17.605s before the full run and passed again as
part of the full run (`internal/measure` in 38.230s). This resolves the one
transient measurement timeout seen on an earlier exact-head run.

## Hosted and post-merge provenance

The direct TUI fixture landed in PR [#342](https://github.com/saiaathishkarthik/picogent/pull/342)
from source `715bf66a28cb4c12a1fa6b37b77b9dd80001e41c`. Its hosted Ubuntu,
Windows, macOS, security, and `release-evidence` jobs passed in run
`33676070896`, and the merge commit is the exact source head recorded above.

The reconciliation PR for this document records its own source SHA, hosted
checks, merge commit, and post-merge main CI in [#338](https://github.com/saiaathishkarthik/picogent/issues/338),
alongside the related parent issues [#311](https://github.com/saiaathishkarthik/picogent/issues/311)
and [#246](https://github.com/saiaathishkarthik/picogent/issues/246).

## Limits

The evidence does not prove:

- rendered terminal behavior or rendered browser behavior;
- Windows console-control or child-process signal semantics;
- arbitrary crash windows between non-atomic application operations;
- recovery from hostile or uncooperative same-UID filesystem writers;
- pathname/TOCTOU race resistance;
- live-provider quality, multi-hour stability, or release readiness.

Those boundaries remain `UNVERIFIED` until directly observed with the
appropriate platform, rendered surface, provider, or adversarial harness.
