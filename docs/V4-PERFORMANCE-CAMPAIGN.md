# V4 performance campaign

Status: historical comparison captured on 2026-08-25, with current-head
refreshes captured on 2026-08-30. This document records deterministic local
controls; it does not claim live-provider quality or end-to-end product
performance.

The measurements below retain the exact historical heads named in each
section. Current merged `main` is now
`cfa61784d706eb090c99c2286632af266608c71a`; the older refresh at
`eb824f293255d913dca894228bbd23609f37fe96` is intentionally not relabeled.
Issue #302 tracks the current scripted-edit performance follow-up.

## Comparison

- Host: Apple M3 arm64 macOS
- Go: `go1.26.6`
- v3 checkout: `a07943b31044049afb0142f39198244cd3c75218`
- v4 candidate: `aaebaed` (`perf: restore compact context and verification planning`)
- Benchmark fixture: `internal/benchmark/benchmark_test.go`
- Repetitions: `-benchtime=100ms -benchmem -count=3`, except scripted edit
  (`-benchtime=1x`)

Commands:

```sh
go test ./internal/benchmark -run '^$' \
  -bench '^(BenchmarkContextManage|BenchmarkRepoMap|BenchmarkSession|BenchmarkVerification)' \
  -benchtime=100ms -benchmem -count=3
go test ./internal/benchmark -run '^$' \
  -bench '^BenchmarkScriptedAgentEdit$' -benchtime=1x -benchmem -count=3
```

## Results

Ranges below are the three observed runs. `B/op` and `allocs/op` are local
regression signals, not product SLAs.

| Operation | v3 time | v4 time | v3 memory / allocs | v4 memory / allocs |
| --- | ---: | ---: | ---: | ---: |
| Context manage, working set | 41.095–42.543 µs | 38.004–39.264 µs | 105,066–105,069 B / 243 | 67,460–67,462 B / 185 |
| Context manage, context-heavy | 2.355–2.469 ms | 2.393–2.615 ms | 485,575–486,945 B / 658–660 | 485,494–486,725 B / 658–660 |
| Repo-map inspect | 26.061–56.334 ms | 21.427–25.384 ms | 49,846–52,414 B / 249–250 | 56,446–59,736 B / 313–315 |
| Repo-map format | 2.665–2.759 µs | 2.842–4.632 µs | 2,370 B / 12 | 2,386 B / 12 |
| Session metadata list, 60 records | 12.375–29.919 ms | 9.192–23.175 ms | 183,168–183,200 B / 1,962 | 183,168–183,200 B / 1,962 |
| Session load | 281.594–420.159 µs | 488.275–911.160 µs | 3,352–3,368 B / 37 | 3,368 B / 37 |
| Verification plan | 1.897–1.924 µs | 1.967–2.142 µs | 864 B / 15 | 864 B / 15 |
| Verification evidence status | 3.315–3.557 µs | 3.475–3.613 µs | 1,792 B / 1 | 1,792 B / 1 |
| Scripted edit turn | 1.068–2.016 ms | 0.759–0.990 ms | 113,328–116,176 B / 1,114–1,150 | 113,328–116,336 B / 1,114–1,151 |

Earlier v4-only additions, before the current-head refresh:

- Repository provenance capture: `39.449–128.307 ms`, `71,637–84,348 B`,
  `430–433 allocs`.
- Snapshot formatting: `4.500–4.532 µs`, `3,700–3,702 B`, `13 allocs`.
- Verification manifest projection: `4.685–4.997 µs`, `3,669 B`, `7 allocs`,
  `1,001 bytes/op`.

## Binary, startup, and RSS snapshot

Built from the two comparison SHAs with `go build -o <path> ./cmd/picogent`:

| Signal | v3 | v4 |
| --- | ---: | ---: |
| Binary size | 16,745,394 B | 16,814,834 B |
| `picogent version`, first observed invocation | 28.375 ms | 27.178 ms |
| `picogent version`, four warm observed invocations | 5.145–6.634 ms | 5.156–5.854 ms |
| Maximum RSS, one `version` invocation | 14,155,776 B | 13,926,400 B |
| Peak memory footprint, same invocation | 5,669,416 B | 5,472,832 B |

Startup and RSS are process-level `version` snapshots, not GUI/browser startup
or a sustained session envelope. They are useful smoke signals only; repeat
on supported platforms and with the GUI/TUI/headless paths before treating
them as release budgets.

Filesystem-backed repository and session measurements have wide run-to-run
variance. The repo-map inspect v3 range includes one 56 ms run; the session
list and load ranges are likewise not stable enough to establish a product
regression from this campaign alone.

## Regression diagnosis and repair

Before `aaebaed`, v4 on the same host measured:

- Working-set context: `76.763–78.015 µs`, `182,369–182,381 B`, `446 allocs`.
- Verification plan: `5.458–5.607 µs`, `1,792 B`, `21 allocs`.

The first regression came from failure-signal digestion doing work even when
stale output contained no signal: lowercasing, `Fields`/`Join`, a candidate
slice, and later ranking structures. The repaired path keeps the same
case-insensitive signal behavior, but avoids those allocations when no signal
exists and scans only the bounded prefix needed for ordinary single-space
lines. Signal-bearing output remains flattened, ranked, deduplicated, and
bounded. Focused `internal/ctxmgr` tests pass.

The verification regression came from calling `os.Stat` for every target,
including known `.go` files. The repaired path maps `.go` targets directly and
still stats dotted non-Go paths so real dotted directories remain valid package
targets. Focused verification tests cover both non-Go files and dotted
directories.

## Interpretation

The repaired working-set path materially beats the measured v3 control on this
fixture. Verification planning returns to v3-scale time and allocation cost.
Context-heavy work remains approximately flat with overlapping ranges. Other
v4 additions have measurable cost, especially provenance capture; that cost is
recorded rather than hidden. No broad claim that v4 is faster is justified.

## Historical current-head microbenchmark refresh

The focused benchmark subset was rerun on the exact historical merged v4 head
`eb824f293255d913dca894228bbd23609f37fe96` on an Apple M3 arm64 Mac with Go
`go1.26.6`. It used the same `-benchtime=100ms -benchmem -count=3` settings as
the comparison above. The v3 values remain the clean-baseline measurements
recorded on `a07943b31044049afb0142f39198244cd3c75218`.

| Operation | v3 time | v4 time | v3 memory / allocs | v4 memory / allocs |
| --- | ---: | ---: | ---: | ---: |
| Context manage, working set | 41.933–44.446 µs | 34.226–34.356 µs | 105,065–105,072 B / 243 | 67,461–67,463 B / 185 |
| Context manage, context-heavy | 2.288–2.333 ms | 2.194–2.331 ms | 485,922–486,491 B / 658–659 | 486,270–487,552 B / 659–661 |
| Repo-map inspect | 22.602–23.494 ms | 34.236–104.401 ms | 48,702–54,760 B / 249–250 | 125,830–149,224 B / 346–370 |
| Repo-map capture | — | 25.268–25.480 ms | — | 125,562–134,690 B / 351–353 |
| Repo-map format | 2.786–2.802 µs | 28.515–29.395 µs | 2,370 B / 12 | 2,551–2,552 B / 20 |
| Session metadata list, 60 records | 7.051–8.619 ms | 8.490–11.155 ms | 183,168–183,200 B / 1,962 | 181,786–185,151 B / 1,482 |
| Session load, canonical | 198.876–526.745 µs | 295.781–449.590 µs | 3,368 B / 37 | 4,352–4,445 B / 41 |
| Session load, legacy history | — | 2.799–3.344 ms | — | 2,454,667–2,464,960 B / 1,549–1,552 |
| Verification plan | 1.895–2.027 µs | 1.980–2.056 µs | 864 B / 15 | 864 B / 15 |
| Verification evidence status | 3.434–3.450 µs | 3.186–3.292 µs | 1,792 B / 1 | 1,792 B / 1 |
| Verification manifest | — | 4.551–4.664 µs | — | 3,669 B / 7; 1,001 bytes/op |

The current head retains the working-set and verification improvements. The
canonical session load now has a separate 41-allocation measurement, while
legacy histories intentionally remain on the full retention path. Repository
capture now reuses one porcelain-v2 status projection for the full head,
branch, dirty counters, and workspace-scoped dirty paths; the current sample
is about 351–353 allocations and 126–135 KB per operation. Filesystem timing
still varies substantially across runs, so the allocation reduction is the
reliable signal and no broad latency claim is justified.

## Current-head long-horizon composition probe

The earlier measurement used `origin/main` head
`cb69c4bb820f49d10f49d702e3ee061ca4a22ec2` and recorded a deterministic
composition probe in
`internal/agent/long_horizon_test.go`. It drives 96 logical turns, saves and
reloads the session and task state on every turn, runs context management, and
changes the intent once midway through the run. The fixture intentionally
exceeds the session, turn, and evidence retention rings; it is a durability
and cost envelope, not a live-provider or GUI benchmark.

Host: Apple M3 arm64 macOS. Command:

```sh
go test ./internal/agent -run '^$' \
  -bench '^BenchmarkLongHorizonResumeEnvelope$' \
  -benchtime=1x -benchmem -count=3
```

| Signal | Three observed runs |
| --- | ---: |
| 96-turn save/reload envelope | 3.369–3.615 s/op |
| Allocated bytes | 918.9–920.4 MB/op |
| Allocations | 1,768,233–1,768,554/op |
| Retained session messages | 128/op |
| Retained task turns / evidence | 16 / 16 per op |
| Durable context peak | 1,493 chars/op |
| Managed context peak | 632 tokens/op |
| Session / task JSON peak | 26,563 / 8,506–8,512 bytes/op |

The higher current-head allocation cost is a useful regression signal and
motivates a separate optimization experiment; it is not a release budget. The
test also covers a bounded fresh-process attachment through
`Agent.SetTaskSession`: an active durable turn is loaded, marked interrupted
with `recover` / `process_restart` / `UNVERIFIED` metadata, and persisted by the
new process. This is not hostile process-death proof. Sustained RSS,
live-provider quality, rendered surfaces, and v3-v4 comparative quality remain
unverified.

## Current-head process-envelope harness

The process-level gap now has a deterministic test harness in
`internal/agent/process_envelope_test.go`:

```sh
go test ./internal/agent -run '^TestLongHorizonProcessEnvelope$' -count=1 -v
```

The parent starts a fresh copy of the test binary with a minimal deterministic
environment, waits for an explicit readiness marker, and then releases a
96-turn child workload. The child reuses the canonical long-horizon fixture
and emits one checkpoint only after each durable save/reload boundary. The
parent samples the child resident set at those checkpoints, verifies all 96
checkpoints and a clean child exit, and bounds failure diagnostics plus child
runtime with a 45-second timeout.

Resident-set sources are platform-specific but the logged metric is always
`resident_set` in bytes: Linux reads `/proc/<pid>/status` `VmRSS` with a `ps`
fallback, other Unix targets use `ps -o rss=`, and Windows uses
`GetProcessMemoryInfo` `WorkingSetSize`. The log records platform, source,
unit, horizon, sample count, unavailable samples, and peak growth. A sample
that races with normal process exit is reported as `availability=partial`,
not as zero.

Observed locally on the exact process-harness checkpoint `31422b7` (Apple M3
arm64 macOS, Go `go1.26.6`): 96 checkpoints, 96 resident-set samples,
3.370-second child workload, 13,565,952-byte initial resident set, and
9,027,584-byte peak growth. This is a runner-specific observation, not a
cross-platform comparison or a release budget. Hosted Windows and Linux
measurements, GUI/TUI/headless envelopes, live-provider behavior, and
long-horizon product quality remain unverified.

## Not measured here

- cross-platform RSS ranges, a stable release budget, and GUI/TUI/headless
  process envelopes;
- live provider latency, token billing, model-call quality, or completion rate;
- rendered GUI/TUI/headless journey latency;
- browser, network, external research, and cross-platform runtime performance;
- a CI-enforced performance budget.

Those require separate evidence. This campaign must be updated before a final
release claim if those measurements remain release-critical.
