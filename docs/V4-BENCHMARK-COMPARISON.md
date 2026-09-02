# Deterministic v3/v4 benchmark comparison

Status: measured locally on 2026-09-02. This is a provider-independent
regression comparison, not a live-provider quality, browser, or product-SLA
claim.

The comparison table is anchored to the current merged `main` head
`275afdb8bdb727ce7a67d37a0b4570eea595f125`. Issue #302 tracks the scripted-edit
follow-up, and `internal/benchmark/stages_test.go` provides stage controls for
that investigation.

## Reproducibility

The same benchmark commands were run three times at both exact heads, on the
same Apple M3 arm64 Mac with Go `go1.26.6`:

- v3 baseline: `a07943b31044049afb0142f39198244cd3c75218`
- v4 candidate: `275afdb8bdb727ce7a67d37a0b4570eea595f125`

Deterministic benchmark group:

```sh
go test ./internal/benchmark -run '^$' \
  -bench '^(BenchmarkContextManage|BenchmarkRepoMap|BenchmarkSession|BenchmarkVerification)' \
  -benchtime=100ms -benchmem -count=3
```

Scripted edit benchmark:

```sh
go test ./internal/benchmark -run '^$' \
  -bench '^BenchmarkScriptedAgentEdit$' -benchtime=1x -benchmem -count=3
```

Both commands passed at both heads. The tables below report the median and the
minimum/maximum from the three runs. Times are `ns/op`; allocation columns are
`B/op` and `allocs/op`.

## Common benchmark operations

| Operation | v3 time (min / median / max) | v4 time (min / median / max) | v3 B/op | v4 B/op | v3 allocs/op | v4 allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Context manage, working set | 44,203 / 44,309 / 44,485 | 37,365 / 37,545 / 37,781 | 105,065 / 105,067 / 105,068 | 67,460 / 67,464 / 67,466 | 243 | 185 |
| Context manage, context-heavy | 2,272,981 / 2,279,984 / 2,286,312 | 2,298,962 / 2,330,979 / 2,331,057 | 485,490 / 485,913 / 486,840 | 485,467 / 486,404 / 486,886 | 658 / 658 / 660 | 658 / 659 / 660 |
| Repo-map inspect | 22,564,575 / 22,712,867 / 24,842,950 | 18,370,188 / 18,996,340 / 21,045,910 | 50,169 / 50,179 / 54,483 | 121,102 / 123,745 / 124,706 | 255 / 256 / 260 | 297 / 298 / 299 |
| Repo-map format | 2,779 / 2,781 / 2,782 | 28,883 / 29,062 / 29,125 | 2,370 | 2,542 / 2,551 / 2,552 | 12 | 20 |
| Session metadata list, 60 records | 5,514,927 / 5,891,787 / 6,209,627 | 30,808,229 / 32,921,448 / 34,540,156 | 183,200 / 183,200 / 183,206 | 231,396 / 231,420 / 231,424 | 1,962 | 3,069 / 3,069 / 3,069 |
| Verification plan | 1,987 / 2,054 / 2,065 | 1,933 / 1,944 / 1,992 | 864 | 864 | 15 | 15 |
| Verification evidence | 3,495 / 3,498 / 3,609 | 3,436 / 3,438 / 3,447 | 1,792 | 1,792 | 1 | 1 |

## Changed benchmark shape

The v4 benchmark split session loading into canonical and legacy-history
fixtures, so those rows are not a strict one-to-one comparison with the v3
`SessionLoad` fixture:

| Operation | v3 time (min / median / max) | v4 time (min / median / max) | v3 B/op | v4 B/op | v3 allocs/op | v4 allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Session load | 185,411 / 188,009 / 217,628 | — | 3,336 / 3,368 / 3,368 | — | 37 | — |
| Session load, canonical | — | 1,015,687 / 1,065,275 / 1,290,865 | — | 6,040 / 6,410 / 6,780 | — | 92 |
| Session load, legacy-history | — | 3,973,399 / 4,030,560 / 4,272,300 | — | 2,455,952 / 2,463,143 / 2,463,411 | — | 1,599 / 1,599 / 1,600 |

V4 also measures new operations with no v3 counterpart:

| Operation | v4 time (min / median / max) | v4 B/op | v4 allocs/op |
| --- | ---: | ---: | ---: |
| Repo-map capture | 18,257,340 / 20,019,639 / 21,044,425 | 118,851 / 119,754 / 123,561 | 302 / 304 / 304 |
| Repo-map snapshot format | 46,649 / 46,750 / 46,825 | 3,912 / 3,913 / 3,913 | 22 |
| Verification manifest | 4,862 / 4,887 / 4,897 | 3,668 / 3,669 / 3,669 | 7 |

## Scripted edit benchmark

This fixture performs one deterministic edit turn per repetition and reports
the model-call count supplied by the test double. It does not measure a live
model:

| Operation | v3 time (min / median / max) | v4 time (min / median / max) | v3 B/op | v4 B/op | v3 allocs/op | v4 allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Scripted agent edit | 551,042 / 589,875 / 1,155,500 | 7,371,376 / 7,559,750 / 9,845,291 | 113,328 / 113,920 / 116,288 | 125,376 / 125,632 / 127,936 | 1,114 / 1,116 / 1,151 | 1,316 / 1,316 / 1,351 |

## Current-main stage controls

At exact `main` head `275afdb8bdb727ce7a67d37a0b4570eea595f125`, the new
provider-independent stage controls were run on the same Apple M3 arm64 host
with Go `go1.26.6`:

```sh
go test ./internal/benchmark -run '^$' \
  -bench '^BenchmarkScriptedAgentEditStage' -benchtime=100ms -benchmem -count=3
```

The first five rows are standalone primitive attribution controls. The final
row is a production-shaped durable turn: it runs `Agent.Run` with workspace-
local task storage, the real pre-publication undo hook, durable mutation and
turn persistence, and the normal turn close. It is still not a replacement for
the end-to-end scripted edit fixture or a live-provider measurement.

| Stage | Time range | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| Project run lock acquire/release | 1.310–1.357 ms | 2,984–3,016 | 63 |
| Checkpoint capture | 0.616–0.620 ms | 6,376–6,392 | 70 |
| Checkpoint seal | 0.554–0.567 ms | 1,828–1,830 | 24 |
| Secure workspace publication | 3.391–4.523 ms | 1,736–1,759 | 39 |
| Secure workspace publication with undo hook | 4.001–4.524 ms | 1,960–1,976 | 40–41 |
| Durable task save | 5.327–5.760 ms | 11,193–11,208 | 148 |
| Production-shaped durable scripted turn | 39.544–41.968 ms | 333,376–347,333 | 3,004–3,014 |

The durable scripted-turn row reports `2.000 model-calls/op`; the primitive
controls do not call a model.

## Interpretation

- Context management improved in the working-set fixture at the median, with
  fewer bytes and allocations; the context-heavy fixture was modestly slower.
- Repo-map inspection was faster at the median, but its bytes and allocation
  counts increased. Repo-map formatting and session metadata listing were
  materially slower in this run.
- The current scripted-edit fixture is materially slower in v4 (about 12.8x at
  the median), with higher bytes and allocations. This is a local regression
  signal to profile; it is not evidence that live-provider quality regressed.
- Verification plan/evidence remained close to the v3 baseline. The v4-only
  manifest path adds measurable work that has no v3 counterpart.

These measurements establish a reproducible comparison and identify follow-up
performance work. They do not justify a broad v4 performance or quality claim;
live providers, rendered surfaces, external research, and long-horizon product
outcomes remain outside this fixture.
