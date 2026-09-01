# Deterministic v3/v4 benchmark comparison

Status: measured locally on 2026-08-31. This is a provider-independent
regression comparison, not a live-provider quality, browser, or product-SLA
claim.

The comparison table is anchored to the historical v4 checkpoint
`6a0126d46cbe6720fbcb54f8b30652160e8cb5`; it is not a claim about the current
`main`. The current merged `main` head for this update is
`cfa61784d706eb090c99c2286632af266608c71a`. Issue #302 tracks the scripted-edit
follow-up, and `internal/benchmark/stages_test.go` provides stage controls for
that investigation.

## Reproducibility

The same benchmark commands were run three times at both exact heads, on the
same Apple M3 arm64 Mac with Go `go1.26.6`:

- v3 baseline: `a07943b31044049afb0142f39198244cd3c75218`
- v4 checkpoint: `6a0126d46cbe6720fbcb54f8b30652160e8cb5`

Deterministic benchmark group:

```sh
go test ./internal/benchmark -run '^$' \
  -bench '^(BenchmarkContextManage|BenchmarkRepoMap|BenchmarkSession|BenchmarkVerification)' \
  -benchtime=100ms -benchmem -count=3
```

Scripted edit checkpoint:

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
| Context manage, working set | 45,825 / 45,895 / 45,919 | 34,636 / 35,001 / 83,580 | 105,068 / 105,069 / 105,070 | 67,458 / 67,461 / 67,461 | 243 | 185 |
| Context manage, context-heavy | 2,353,805 / 2,371,696 / 2,371,784 | 2,223,283 / 2,244,877 / 2,259,434 | 485,925 / 486,404 / 487,850 | 485,837 / 486,674 / 487,810 | 658 / 659 / 662 | 658 / 660 / 662 |
| Repo-map inspect | 23,797,767 / 24,943,925 / 26,309,700 | 20,738,383 / 22,070,717 / 22,154,317 | 43,531 / 45,360 / 45,488 | 119,049 / 119,195 / 119,899 | 304 / 305 / 305 | 342 / 343 / 345 |
| Repo-map format | 2,968 / 3,013 / 3,092 | 29,547 / 29,608 / 29,661 | 2,370 | 2,639 / 2,639 / 2,674 | 12 | 20 |
| Session metadata list, 60 records | 8,784,321 / 9,687,413 / 14,034,917 | 41,935,778 / 42,492,500 / 45,649,806 | 189,936 / 189,998 / 190,016 | 240,341 / 252,826 / 253,029 | 1,962 | 3,130 / 3,131 / 3,131 |
| Verification plan | 1,852 / 1,869 / 1,882 | 1,878 / 1,883 / 1,887 | 896 / 912 / 912 | 896 / 912 / 912 | 15 | 15 |
| Verification evidence | 3,189 / 3,192 / 3,195 | 3,257 / 3,285 / 3,354 | 1,792 | 1,792 | 1 | 1 |

## Changed benchmark shape

The v4 benchmark split session loading into canonical and legacy-history
fixtures, so those rows are not a strict one-to-one comparison with the v3
`SessionLoad` fixture:

| Operation | v3 time (min / median / max) | v4 time (min / median / max) | v3 B/op | v4 B/op | v3 allocs/op | v4 allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Session load | 290,224 / 291,331 / 316,807 | — | 3,512 | — | 37 | — |
| Session load, canonical | — | 1,377,795 / 1,399,341 / 1,507,897 | — | 6,280 / 6,280 / 6,755 | — | 94 |
| Session load, legacy-history | — | 3,925,110 / 3,953,906 / 3,983,201 | — | 2,466,630 / 2,468,743 / 2,469,742 | — | 1,602 / 1,603 / 1,604 |

V4 also measures new operations with no v3 counterpart:

| Operation | v4 time (min / median / max) | v4 B/op | v4 allocs/op |
| --- | ---: | ---: | ---: |
| Repo-map capture | 20,869,867 / 21,145,333 / 21,780,283 | 114,729 / 115,953 / 118,617 | 347 / 347 / 350 |
| Repo-map snapshot format | 48,736 / 48,842 / 49,135 | 4,042 / 4,043 / 4,043 | 22 |
| Verification manifest | 4,742 / 4,769 / 4,814 | 3,669 / 3,669 / 3,670 | 7 |

## Scripted edit checkpoint

This fixture performs one deterministic edit turn per repetition and reports
the model-call count supplied by the test double. It does not measure a live
model:

| Operation | v3 time (min / median / max) | v4 time (min / median / max) | v3 B/op | v4 B/op | v3 allocs/op | v4 allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Scripted agent edit | 577,208 / 944,291 / 1,229,584 | 6,116,333 / 7,032,417 / 11,119,916 | 128,112 / 128,592 / 131,072 | 133,160 / 133,224 / 135,512 | 1,252 / 1,253 / 1,289 | 1,378 / 1,379 / 1,413 |

## Current-main stage controls

At exact `main` head `cfa61784d706eb090c99c2286632af266608c71a`, the new
provider-independent stage controls were run on the same Apple M3 arm64 host
with Go `go1.26.6`:

```sh
go test ./internal/benchmark -run '^$' \
  -bench '^BenchmarkScriptedAgentEditStage' -benchtime=100ms -benchmem -count=3
```

These are public-primitive attribution controls, not a replacement for the
end-to-end scripted edit fixture. The durable task-save row is not included in
the no-task end-to-end fixture.

| Stage | Time range | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| Project run lock acquire/release | 1.109–1.367 ms | 3,112–3,144 | 65 |
| Checkpoint capture | 0.485–0.527 ms | 7,024–7,040 | 75 |
| Checkpoint seal | 0.601–0.904 ms | 1,882–1,889 | 25 |
| Secure workspace publication | 4.499–4.989 ms | 2,000–2,015 | 41 |
| Durable task save | 7.434–8.181 ms | 11,452–11,552 | 152–153 |

## Interpretation

- Context management improved in the working-set fixture and was modestly
  faster in the context-heavy fixture, with fewer working-set allocations.
- Repo-map inspection was faster at the median, but its bytes and allocation
  counts increased. Repo-map formatting and session metadata listing were
  materially slower in this run.
- The scripted edit checkpoint was materially slower in v4. That is a local
  regression signal to profile; it is not evidence that live-provider quality
  regressed.
- Verification plan/evidence remained close to the v3 baseline. The v4-only
  manifest path adds measurable work that has no v3 counterpart.

These measurements establish a reproducible comparison and identify follow-up
performance work. They do not justify a broad v4 performance or quality claim;
live providers, rendered surfaces, external research, and long-horizon product
outcomes remain outside this fixture.
