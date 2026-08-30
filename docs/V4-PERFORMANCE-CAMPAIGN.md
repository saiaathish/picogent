# V4 performance campaign

Status: measured locally on 2026-08-25. This document records deterministic
local controls; it does not claim live-provider quality or end-to-end product
performance.

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

V4-only additions:

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

## Current-head long-horizon composition probe

The current `origin/main` head (`9104061f59837845fb186e9032b9ea40c60aef67`)
now has a deterministic composition probe in
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
| 96-turn save/reload envelope | 2.574–2.835 s/op |
| Allocated bytes | 795.2–795.8 MB/op |
| Allocations | 459,479–459,696/op |
| Retained session messages | 128/op |
| Retained task turns / evidence | 16 / 16 per op |
| Durable context peak | 1,263 chars/op |
| Managed context peak | 632 tokens/op |
| Session / task JSON peak | 26,579–26,580 / 8,511–8,512 bytes/op |

The measured allocation cost is a useful regression signal and motivates a
separate optimization experiment; it is not a release budget. The test also
covers a cooperative fresh-process restart: an active durable turn is loaded,
marked interrupted, and persisted by a new process. Hostile process death,
sustained RSS, live-provider quality, rendered surfaces, and v3-v4 comparative
quality remain unverified.

## Not measured here

- binary size, cold/warm startup, RSS, and long-session RSS growth;
- live provider latency, token billing, model-call quality, or completion rate;
- rendered GUI/TUI/headless journey latency;
- browser, network, external research, and cross-platform runtime performance;
- a CI-enforced performance budget.

Those require separate evidence. This campaign must be updated before a final
release claim if those measurements remain release-critical.
