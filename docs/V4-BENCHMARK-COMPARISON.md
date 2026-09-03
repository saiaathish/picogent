# Deterministic v3/v4 benchmark comparison

Status: measured locally on 2026-09-02. This is a provider-independent
regression comparison, not a live-provider quality, browser, or product-SLA
claim.

The v3/v4 comparison table below is anchored to the pre-measurement merged v4
head `275afdb8bdb727ce7a67d37a0b4570eea595f125`. The latest merged `main`
head is `4046d6c807514614cd704b73f9ef8da12eaa1ae1`; the earlier current-main
checkpoint at `c6d87bf5b38e6afb1322e6982d3033357df59819` remains below as
historical evidence. PR #332 added a same-iteration causal control for the
scripted-edit follow-up tracked by #302, and PR #372 refreshes that control at
the latest head. `internal/benchmark/stages_test.go` provides both the
standalone and composite controls.

## Reproducibility

The comparison commands were run three times at both exact heads, on the same
Apple M3 arm64 Mac with Go `go1.26.6`:

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

## Historical current-main stage controls

At the prior exact `main` head `c6d87bf5b38e6afb1322e6982d3033357df59819`,
the provider-independent stage controls were rerun on the same Apple M3 arm64
host with Go `go1.26.6`:

```sh
go test ./internal/benchmark -run '^$' \
  -bench '^BenchmarkScriptedAgentEditStage' -benchtime=100ms -benchmem -count=3
```

The standalone primitive rows are attribution controls. The composite row runs
the project lock, checkpoint capture, secure publication, and checkpoint seal
on the same iteration as an additive non-durable safety subtotal. The durable
row is a production-shaped turn: it runs `Agent.Run` with workspace-local task
storage, the real pre-publication undo hook, durable mutation and turn
persistence, and the normal turn close. These remain controls, not a
replacement for a live-provider measurement.

| Stage | Time range | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| Project run lock acquire/release | 1.190–1.330 ms | 2,984–3,016 | 63 |
| Checkpoint capture | 0.612–0.640 ms | 6,392 | 70 |
| Checkpoint seal | 0.550–0.560 ms | 1,829–1,830 | 24 |
| Secure workspace publication | 3.861–4.122 ms | 1,736–1,749 | 39 |
| Secure workspace publication with undo hook | 3.949–4.707 ms | 1,973–1,991 | 40 |
| Non-durable composite safety subtotal | 6.093–6.941 ms | 13,025–13,083 | 197–198 |
| Durable task save | 5.698–5.831 ms | 11,212–11,281 | 148–149 |
| Production-shaped durable scripted turn | 40.913–41.905 ms | 319,690–349,482 | 3,002–3,020 |

The durable scripted-turn row reports `2.000 model-calls/op`; the primitive and
composite controls do not call a model. This section is retained as a
historical checkpoint and is not the latest-main result.

## Historical current-main causal scripted-edit follow-up

At the same historical merged `main` head
`c6d87bf5b38e6afb1322e6982d3033357df59819`,
the full non-durable scripted-edit fixture and its same-iteration composite
control were run three times with `-benchtime=100ms -benchmem -count=3`:

| Operation | Time (min / median / max) | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| Full `Agent.Run` scripted edit | 6,075,236 / 6,816,022 / 7,002,236 ns/op | 125,346–125,674 | 1,312 |
| Same-iteration composite safety subtotal | 6,305,433 / 6,723,530 / 6,917,500 ns/op | 13,058–13,091 | 197–198 |

The full fixture reports `2.000 model-calls/op`. The composite phase ranges on
this run were run-lock `1.286–1.368 ms`, capture `0.671–0.693 ms`, secure
publication `3.524–3.950 ms`, and seal/changed-path `0.824–0.907 ms`. The
subtotal accounts for essentially all of this provider-independent local
fixture's wall time; this does not establish a live-provider regression,
product SLA, or release authorization.

## Fresh current-main scripted-edit checkpoint

At exact merged `main` head `4046d6c807514614cd704b73f9ef8da12eaa1ae1`, the
same provider-independent controls were rerun on an Apple M3 arm64 Mac with
Go `go1.26.6` on 2026-09-02. The repeated command was:

```sh
go test ./internal/benchmark -run '^$' \
  -bench 'BenchmarkScriptedAgentEdit($|Stage)' -benchtime=100ms -benchmem -count=3
```

The full fixture and the same-iteration safety controls reported:

| Operation | Time (min / median / max) | B/op | allocs/op | model-calls/op |
| --- | ---: | ---: | ---: | ---: |
| Full `Agent.Run` scripted edit | 7,184,314 / 7,635,533 / 9,355,554 ns/op | 141,621–141,674 | 1,463 | 2.000 |
| Non-durable composite safety subtotal | 8,033,003 / 9,320,351 / 9,610,310 ns/op | 13,962–13,994 | 207 | — |
| Production-shaped durable scripted turn | 65,744,208 / 72,648,020 / 99,701,250 ns/op | 367,680–393,576 | 3,214–3,239 | 2.000 |

The standalone stage controls reported these ranges:

| Stage | Time range | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| Project run lock acquire/release | 1.273–1.974 ms | 3,143–3,144 | 65 |
| Checkpoint capture | 0.608–0.668 ms | 7,072 | 75 |
| Checkpoint seal | 0.609–0.683 ms | 1,886–1,889 | 25 |
| Secure workspace publication | 5.011–5.856 ms | 1,792–1,811 | 40 |
| Secure workspace publication with undo hook | 5.495–6.173 ms | 2,016–2,068 | 41–42 |
| Durable task save | 7.724–7.904 ms | 11,495–11,545 | 152–153 |

The composite's same-iteration phase ranges were run-lock `1.532–1.969 ms`,
capture `0.828–0.872 ms`, secure publication `4.605–5.121 ms`, and
seal/changed-path `1.068–1.767 ms`. The composite profile is dominated by
kernel `openat` calls across the descriptor-safe lock, capture, and publication
boundaries. That is a measurement of the required safety machinery, not proof
of a redundant operation that can be removed or shared safely. The run remains
a local control: it does not establish live-provider quality, cross-platform
runtime behavior, a product SLA, or release readiness. No new production
optimization lane is justified by this checkpoint alone; #302 remains open for
a separately causal target.

## Interpretation

- Context management improved in the working-set fixture at the median, with
  fewer bytes and allocations; the context-heavy fixture was modestly slower.
- Repo-map inspection was faster at the median, but its bytes and allocation
  counts increased. Repo-map formatting and session metadata listing were
  materially slower in this run.
- The pre-#332 scripted-edit comparison remains materially slower in v4 (about
  12.8x at the median), with higher bytes and allocations. The historical and
  fresh current-main composite controls both show that the existing safety
  boundaries account for essentially all of this provider-independent fixture;
  this is not evidence that live-provider quality regressed.
- Verification plan/evidence remained close to the v3 baseline. The v4-only
  manifest path adds measurable work that has no v3 counterpart.

These measurements establish a reproducible comparison and preserve a bounded
follow-up record for #302. They do not justify a broad v4 performance or
quality claim; live providers, rendered surfaces, external research, and
long-horizon product outcomes remain outside this fixture.
