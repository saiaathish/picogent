# Picogent v3 benchmark suite

This is the reproducible local baseline for v3 performance work. It measures
deterministic work without a provider, network, browser, or live model. It does
not stand in for end-to-end completion quality.

## Run the baseline

Run the filesystem and parsing baselines with three repetitions:

```sh
go test ./internal/benchmark -run '^$' \
  -bench '^(BenchmarkContextManage|BenchmarkRepoMap|BenchmarkSession|BenchmarkVerification)' \
  -benchtime=100ms -benchmem -count=3
```

The scripted edit baseline intentionally uses one operation per repetition so
it does not hide setup and checkpoint-reset work behind a long benchmark loop:

```sh
go test ./internal/benchmark -run '^$' \
  -bench '^BenchmarkScriptedAgentEdit$' -benchtime=1x -benchmem -count=3
```

Record the machine, Go version, git SHA, command, and complete output with each
comparison. `ns/op`, `B/op`, and `allocs/op` are local regression signals, not
product SLAs.

## Initial baseline

Captured on 2026-08-25 at commit `ec88cdd`, on an Apple M3 arm64 Mac with Go
1.25.0. The ranges below are the three runs from the commands above.

| Operation | Time per op | Allocations | Additional metric |
| --- | ---: | ---: | --- |
| Context manage, working set | 41.7–44.1 µs | 105,068 B / 243 | 12 messages, 592 tokens |
| Context manage, context-heavy | 2.24–2.29 ms | 485,897–486,494 B / 658–659 | 7 messages, 1,437 tokens |
| Repo-map inspect | 22.5–22.6 ms | 51,907–58,704 B / 304–307 | deterministic 4-file fixture |
| Repo-map format | 2.68–2.69 µs | 2,370 B / 12 | about 200 MB/s |
| Session metadata list, 60 records | 8.8–21.4 ms | 183,168 B / 1,962 | 60 records |
| Session load | 0.50–0.90 ms | 3,352–3,368 B / 37 | 4 messages |
| Verification plan | 1.87–1.93 µs | 864 B / 15 | targeted + broader plan |
| Verification evidence status | 3.31–3.35 µs | 1,792 B / 1 | exact PASS token |
| Scripted edit turn | 0.59–0.77 ms | 113,808–116,176 B / 1,115–1,150 | 2 model calls; undo checkpoint |

The session-list spread is filesystem-sensitive. Repeat it before optimizing;
one fast run is not evidence of a product regression.

## Scenario matrix

`MEASURED` means the current repository has a deterministic benchmark or
acceptance test. `UNRECORDED` means the scenario is specified but no claim is
made yet. This prevents local mocks from being reported as live-agent quality.

| Scenario | Current evidence | Metrics to record |
| --- | --- | --- |
| Simple edit | MEASURED: `BenchmarkScriptedAgentEdit` | completion, model calls, time, allocations, undo |
| Bug fix | MEASURED: `internal/acceptance/TestV02ReleaseLoop` | correctness, verification, changed files, undo |
| Multi-file feature | UNRECORDED | completion, correctness, files, tool calls, tokens |
| Unfamiliar repository | UNRECORDED | first useful action, questions, map/search time |
| Failing tests | MEASURED locally by verification tests; agent flow UNRECORDED | repair attempts, final evidence, regressions |
| UI improvement | UNRECORDED live browser run | cold load, interaction latency, focus/errors |
| Vague beginner request | UNRECORDED | questions, inferred scope, completion, clarity |
| Refactor | UNRECORDED | correctness, unnecessary changes, broader verification |
| Security issue | MEASURED by security tests; agent scenario UNRECORDED | blocked actions, safe paths, evidence |
| Environment failure | MEASURED by inconclusive verification tests | exit state, explanation, recovery guidance |
| Long task | MEASURED local proxy: context-heavy manage | context growth, compaction, model calls |
| Resume | MEASURED: session list/load benchmarks and TUI tests | discovery, load time, state retained |
| Undo | MEASURED: scripted edit + agent undo tests | restore correctness, conflict behavior, time |
| Cancellation | MEASURED by agent cancellation tests; latency UNRECORDED | cancellation latency, state, resumeability |
| Context-heavy task | MEASURED local proxy; live provider UNRECORDED | tokens, compaction, quality, time |

## Interpretation rules

- Keep v2 and v3 commands, fixtures, machine class, and repetition settings the
  same before comparing results.
- Treat provider, network, browser, and OS measurements as separate evidence
  classes. A scripted client proves local control flow, not model quality.
- Optimize only after a repeated regression or a user-visible bottleneck is
  recorded. Keep the benchmark fixture small enough to run in CI.
- Do not add a daemon, watcher, index, or always-on telemetry for this suite.

The next measurement wave should add a bounded headless scenario runner and a
real browser-based GUI smoke report, then fill the `UNRECORDED` cells only when
their evidence is available.
