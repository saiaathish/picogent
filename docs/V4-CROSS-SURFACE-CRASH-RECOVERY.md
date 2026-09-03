# v4 cross-surface crash-recovery evidence

Status: the conditional L lane in [#338](https://github.com/saiaathish/picogent/issues/338) has direct local macOS evidence at the exact historical source head `f29fa074c468b96badc2f951a76f1f1a90c78b1f`. The evidence covers the existing headless and agent process boundaries plus the GUI and TUI process-kill fixtures. Follow-up Windows console-control checkpoints add hosted evidence for the headless, TUI, and GUI server-shutdown signal fixtures at their platform-appropriate seams. This report is synchronized to current `main` `72f79797e555e38f29d926ed19142d4adae31eaa` by documentation-only checkpoint [#391](https://github.com/saiaathish/picogent/issues/391); fixture observations remain bound to their original source heads and hosted runs. This remains a bounded recovery record, not a release-readiness claim.

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

## Current-main checkpoint — documentation only

The current repository head is `72f79797e555e38f29d926ed19142d4adae31eaa`,
created by merge commit [#390](https://github.com/saiaathish/picogent/pull/390)
from source `eb2b02b8e94f2dab6a82dd937db6b42e4283a63f`. Its post-merge
workflow [33737597716](https://github.com/saiaathish/picogent/actions/runs/33737597716)
passed `security`, Ubuntu, Windows, macOS, and `release-evidence`.

PR #390 changed only the independent release-audit document, so it adds no
new cross-surface runtime observation. The result table below intentionally
retains the source head, merge commit, and hosted run associated with each
original fixture. This checkpoint only binds the report to the exact current
`main` head and preserves the evidence limits.

## Cross-surface result

| Surface | Fixture | Boundary exercised | Result | Direct assertion |
| --- | --- | --- | --- | --- |
| Headless | `TestHeadlessFreshProcessSignalRetainsInterruptedTurn` | Fresh headless child reaches the provider barrier, receives the platform signal (`SIGINT` on Unix; `CTRL_BREAK_EVENT` on Windows), and exits with cancellation | PASS, local macOS; hosted Windows console-control PASS in [PR #377](https://github.com/saiaathish/picogent/pull/377) | The durable task remains retained as interrupted/recover and the completion projection stays fail-closed |
| Agent | `TestLongHorizonResumeAfterProcessKill` | A worker holding the active-turn/run-lock boundary is killed, then a fresh process resumes the same session | PASS, local macOS | The interrupted `process_restart` turn is retained, remains unchanged before follow-up, and the next turn completes without a completion claim |
| GUI | `TestGUIFreshProcessKillRecoversInterruptedTurn` | Real GUI server child reaches an active loopback request and is terminated with `Process.Kill` | PASS, local macOS | Fresh recovery loads the same session as a working interrupted/recover turn with `process_restart` metadata and a not-ready completion projection |
| GUI | `TestGUIFreshProcessShutdownRetainsInterruptedTurn` | Fresh GUI server child reaches the provider barrier and receives the platform signal (`SIGINT` on Unix; `CTRL_BREAK_EVENT` on Windows) | PASS, local macOS; hosted Windows console-control PASS in [PR #386](https://github.com/saiaathish/picogent/pull/386) | The durable task remains retained as interrupted/recover and the completion projection stays fail-closed |
| TUI | `TestTUIFreshProcessKillRecoversInterruptedTurn` | Real TUI model child reaches a durably persisted active turn and is terminated with `Process.Kill` | PASS, local macOS | Fresh `app.LoadContext` plus `newModel` recovers the same session as an interrupted/recover turn with `process_restart` metadata and a not-ready completion projection |
| TUI | `TestTUIFreshProcessSignalRetainsInterruptedTurn` | Real TUI model child reaches the provider barrier and receives the platform signal (`SIGINT` on Unix; `CTRL_BREAK_EVENT` on Windows) | PASS, local macOS; hosted Windows console-control PASS in [PR #379](https://github.com/saiaathish/picogent/pull/379) | The durable task remains retained as interrupted/recover and the completion projection stays fail-closed |

The headless, TUI, and GUI signal fixtures prove the existing cancellation
boundary through the platform-specific console-control helper described above.
The agent and GUI process-kill fixtures prove their existing direct-kill/resume
paths. GUI and TUI use their real server and model/session seams; neither
fixture adds a second lock, store, planner, daemon, watcher, renderer, or
user-facing workflow.

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

The direct TUI fixture landed in PR [#342](https://github.com/saiaathish/picogent/pull/342)
from source `715bf66a28cb4c12a1fa6b37b77b9dd80001e41c`. Its hosted Ubuntu,
Windows, macOS, security, and `release-evidence` jobs passed in run
`33676070896`, and the merge commit is the exact source head recorded above.

The reconciliation PR for this document records its own source SHA, hosted
checks, merge commit, and post-merge main CI in [#338](https://github.com/saiaathish/picogent/issues/338),
alongside the related parent issues [#311](https://github.com/saiaathish/picogent/issues/311)
and [#246](https://github.com/saiaathish/picogent/issues/246).

The later Windows console-control checkpoints are recorded independently:

- Headless [PR #377](https://github.com/saiaathish/picogent/pull/377) used source
  `3923f3251a7b93725cc485e6c60f495c60871bd2`, passed all five hosted gates in
  [run 33721809935](https://github.com/saiaathish/picogent/actions/runs/33721809935),
  merged as `c5c5cf6d27ad275c0f4f0c00ff0817fe36eeea2c`, and passed all five
  gates again in post-merge [run 33722260694](https://github.com/saiaathish/picogent/actions/runs/33722260694).
- TUI [PR #379](https://github.com/saiaathish/picogent/pull/379) used source
  `6ef0d37bd63f3462fa54542ca30fe633224a2925`, passed all five hosted gates in
  [run 33723897413](https://github.com/saiaathish/picogent/actions/runs/33723897413),
  merged as `09fe4b41de4d03dffeb1e69708ae5fd7f45ee412`, and passed all five
  gates again in post-merge [run 33724323436](https://github.com/saiaathish/picogent/actions/runs/33724323436).
- GUI [PR #386](https://github.com/saiaathish/picogent/pull/386) used source
  `6fc4dadae40f52ee7193b2a7ce99ddad39a84614`, passed all five hosted gates in
  [run 33729515961](https://github.com/saiaathish/picogent/actions/runs/33729515961),
  merged as `c54501f03a44b420b61f04762461e19011c7b93e`, and passed all five
  gates again in post-merge [run 33730015632](https://github.com/saiaathish/picogent/actions/runs/33730015632).

The current-main documentation checkpoint is [PR #390](https://github.com/saiaathish/picogent/pull/390),
source `eb2b02b8e94f2dab6a82dd937db6b42e4283a63f`, merged as
`72f79797e555e38f29d926ed19142d4adae31eaa`, with all five jobs passing in
[post-merge run 33737597716](https://github.com/saiaathish/picogent/actions/runs/33737597716).
It changed the independent release audit only and did not rerun or broaden
the cross-surface fixtures documented here.

These are provider-independent test-fixture observations. The Windows helper
creates a process group and delivers `CTRL_BREAK_EVENT`; it does not turn the
direct-process-kill evidence above into a general child-process or crash-window
guarantee.

## Limits

The evidence does not prove:

- rendered terminal behavior or rendered browser behavior;
- rendered signal behavior or GUI child-process signal semantics outside the
  fresh-process fixtures recorded above;
- arbitrary crash windows between non-atomic application operations;
- recovery from hostile or uncooperative same-UID filesystem writers;
- pathname/TOCTOU race resistance;
- live-provider quality, multi-hour stability, or release readiness.

Those boundaries remain `UNVERIFIED` until directly observed with the
appropriate platform, rendered surface, provider, or adversarial harness.
