# V4 exact-head outcome-quality comparison

Status: the prior exact source-pair matrix for [#422](https://github.com/saiaathish/picogent/issues/422) and [#246](https://github.com/saiaathishkarthik/picogent/issues/246) remains preserved in the [2026-09-04 bounded report](V4-OUTCOME-QUALITY-REPORT-2026-09-04.json). The refreshed run for [#435](https://github.com/saiaathish/picogent/issues/435) is recorded in the [2026-09-05 bounded report](V4-OUTCOME-QUALITY-REPORT-2026-09-05.json) and remains `INCONCLUSIVE` because it predates the compatible controlled-transcript metric layer. [#727](https://github.com/saiaathish/picogent/issues/727) defines that layer. The exact-head rerun for [#728](https://github.com/saiaathish/picogent/issues/728), refreshed after [#731](https://github.com/saiaathish/picogent/issues/731)'s artifact-finalization hardening, is preserved in the [2026-09-15 report](V4-OUTCOME-QUALITY-REPORT-2026-09-15.json) and is `complete` for that deterministic measurement boundary only.

The outcome-quality contract and scripted executor are already defined in
[`V4-OUTCOME-QUALITY-BENCHMARK.md`](V4-OUTCOME-QUALITY-BENCHMARK.md). This
document records what the completed matrix observed and what it still cannot
establish. A declared source SHA must correspond to the clean source tree that
executes the target.

## Reproducibility foundation

`ValidateOutcomeQualitySourcePair` accepts one
`OutcomeQualitySourceBinding` for each target. It requires:

- an absolute, resolvable workspace directory;
- distinct baseline and candidate workspaces;
- distinct full source commit IDs;
- matching host, Go, and runner metadata;
- committed `HEAD` equal to each target's declared source SHA; and
- a clean worktree, including untracked and ignored files.

The preflight reuses `verify.CollectProvenance`, so missing Git data, a stale
checkout, or a dirty tree fails closed. It uses bounded, read-only Git
observation and writes no state; the later execution lane owns the build and
child-process protocol.

Example:

```go
err := benchmark.ValidateOutcomeQualitySourcePair(ctx,
    benchmark.OutcomeQualitySourceBinding{
        Target:    baselineTarget,
        Workspace: baselineWorkspace,
    },
    benchmark.OutcomeQualitySourceBinding{
        Target:    candidateTarget,
        Workspace: candidateWorkspace,
    },
)
```

Passing this check proves only that the two source trees were observed at the
declared clean heads. It does not prove that a compiled binary contains that
revision, that a provider was live, or that a task would succeed on an
arbitrary repository.

## Matrix controller and evidence boundary

The bounded controller `RunOutcomeQualitySourcePairMatrix` now composes the
preflight and matrix seams. It accepts one `OutcomeQualitySourceBinding` per
variant plus one executor per variant, validates both source trees before the
first execution, and routes the runner's stable baseline/candidate ordering to
the matching executor. The executors remain separate inputs; the controller
does not share their state or create a second task/report authority.

```go
report, err := benchmark.RunOutcomeQualitySourcePairMatrix(ctx,
    benchmark.OutcomeQualitySourcePairConfig{
        Baseline:  baselineBinding,
        Candidate: candidateBinding,
        Policy:    sharedPolicy,
        Command:   launcherDescription,
    },
    baselineExecutor,
    candidateExecutor,
)
```

The controller still requires the caller to provide executors that were built
or launched from the validated source worktrees. A source-head field returned
by an executor is not binary provenance; target build/launch evidence remains
the next execution responsibility. If a target cannot expose the bounded
measurement adapter, the caller must preserve an inconclusive or `UNVERIFIED`
observation rather than reuse the other variant's worker.

The source-pair execution lane builds or launches each target from the
validated workspace without shell interpolation, sends the same bounded input
and policy to both targets, caps child output and runtime, and validates the
returned observations with `OutcomeQualityReport.Validate()`. A complete
report requires the fixed 20-scenario catalog, at least two repetitions,
deterministic ordering, current verification evidence, and explicit failure or
`UNVERIFIED` reasons.

The current compatible controlled-transcript matrix and the earlier
post-#442 historical matrix are recorded below. Neither establishes
live-provider quality, rendered behavior, arbitrary repository success, release
authorization, or overall v4 readiness. Those claims remain outside this
evidence boundary.

## Current compatible controlled-transcript result

Issue #728's clean exact-head rerun used separate clean source worktrees for
the v3 baseline and current candidate. [#731](https://github.com/saiaathish/picogent/issues/731)
reran that matrix from controller commit
`c90acfbd95b07fba3659b8519779840613de2ca2`; it validates the source pair and
every final evidence condition before persisting the artifact:

```sh
PICOGENT_RUN_EXACT_OUTCOME_QUALITY_MATRIX=1 \
PICOGENT_OUTCOME_QUALITY_REPORT=/private/tmp/picogent-v4-outcome-report-c90acfb.json \
GOMAXPROCS=2 GOFLAGS=-p=1 GOTOOLCHAIN=local \
go test ./internal/benchmark -run '^TestRunOutcomeQualityExactSourcePairMatrix$' -count=1
```

The raw artifact is committed as
[`V4-OUTCOME-QUALITY-REPORT-2026-09-15.json`](V4-OUTCOME-QUALITY-REPORT-2026-09-15.json).
It passed `OutcomeQualityReport.Validate()` in the matrix test and has SHA-256
`62487dddb39df5248f42c6aa3dbf9f4537c2a2b8bece75233eb56c2b0f537b09`.

| Field | Recorded evidence |
| --- | --- |
| Baseline source | `a07943b31044049afb0142f39198244cd3c75218` |
| Candidate source | `c11608747bb82b3e820009ee3f28498931758f12` |
| Matrix controller head | `c90acfbd95b07fba3659b8519779840613de2ca2` |
| Host/toolchain | `darwin/arm64`, `go1.26.6` |
| Runner | `picogent-outcome-quality-runner-v1` |
| Shared policy | 2 repetitions, 30-second observation timeout, 16,000 maximum tokens, 64 maximum model calls, 256 maximum tool calls, 32 maximum turns |
| Coverage | 20 scenarios × 2 variants × 2 repetitions = 80/80 observations |
| Report status | `complete` |

Every observation records `pass` outcome success, correctness, and
verification quality with `current` evidence; none records an unverified
reason. The repair-count range is zero for both variants, so this run supplies
no evidence about repair effectiveness. The recorded controlled-transcript
context-growth ranges are 785–818 bytes for the baseline and 733–765 bytes for
the candidate. Those ranges are fixture measurements, not a broad quality or
performance claim.

The #731 controller retains the same bounded measurement result while making
the artifact itself fail closed: a failed count, metric, provider-request, or
post-run source-pair check writes no report. This is artifact-integrity evidence
only; it does not broaden the deterministic measurement boundary.

This closes the compatible deterministic measurement and provenance boundary
for #728. It does not establish a live-provider quality win, a general
autonomous-coding result, release readiness, or v4 completion.

## Historical matrix result (inconclusive)

The reviewed run used the fixed 20-scenario catalog, baseline-before-candidate
ordering, and two repetitions. It captured all 80 expected observations and
persisted a structurally valid report before the detailed evidence assertions
ran.

The complete per-observation JSON is committed as
[`V4-OUTCOME-QUALITY-REPORT-2026-09-05.json`](V4-OUTCOME-QUALITY-REPORT-2026-09-05.json).
It is a local `darwin/arm64` opt-in execution artifact. PR #436 fixed the
candidate full-fixture proof binding, and [PR #442](https://github.com/saiaathish/picogent/pull/442)
fixed the legacy v3 fixture-mode boundary that previously blocked
`advanced-architecture`. The post-#442 matrix run passed the opt-in test in
355.434 seconds; hosted checks validate the source fixes and do not claim to
have rerun this 80-observation matrix.

| Field | Recorded evidence |
| --- | --- |
| Baseline source | `a07943b31044049afb0142f39198244cd3c75218` |
| Candidate source | `a6d3af39bb24559fe2d71b4063cb1b8411cd2c7e` |
| Matrix test-anchor head | `7137681` |
| Merge/current `main` at the run | `a6d3af39bb24559fe2d71b4063cb1b8411cd2c7e` |
| Host/toolchain | `darwin/arm64`, `go1.26.6` |
| Runner | `picogent-outcome-quality-runner-v1` |
| Shared policy | 2 repetitions, 30-second observation timeout, 32 maximum turns |
| Coverage | 20 scenarios × 2 variants × 2 repetitions = 80/80 observations |
| Report status | `inconclusive` |

The recorded result is not a v4 quality win or regression claim:

| Variant | Observations | Observed boundary |
| --- | ---: | --- |
| v3 baseline | 40 | All remain `INCONCLUSIVE` because the historical report used the prior adapter, which lacked compatible repair-count/context-growth telemetry; both `advanced-architecture` observations reached the same boundary without a fixture-write failure. |
| v4 candidate | 40 | All passed the deterministic fixture with current verification, including the required full three-file capture. |
| Comparison | 80 | No v3/v4 quality delta is claimable because the baseline telemetry boundary remains incomplete; candidate fixture proof is complete and the prior baseline fixture failures are gone. |

The catalog contains the following scenario counts. Counts are coverage
counts, not successful outcomes; every row remains `INCONCLUSIVE` for
comparison purposes.

| Category | Scenarios | Observations | Interpretation |
| --- | ---: | ---: | --- |
| Beginner | 3 | 12 | No comparable pass/fail delta recorded. |
| Standard development | 4 | 16 | No comparable pass/fail delta recorded. |
| Advanced | 4 | 16 | No comparable pass/fail delta; both baseline `advanced-architecture` observations reached the normal unsupported-telemetry boundary. |
| Product | 3 | 12 | No comparable pass/fail delta recorded. |
| Robustness | 5 | 20 | No comparable pass/fail delta recorded. |
| Long horizon | 1 | 4 | No comparable pass/fail delta recorded. |

## Decision and next evidence boundary

The current #728 artifact is a complete compatible measurement and provenance
checkpoint for the deterministic lane, not a quality-improvement result. It
does not relabel the historical `INCONCLUSIVE` report: that report retains its
prior adapter boundary and the post-#442 baseline fixture-write repair history.
The new report may support later, separately designed comparisons, but it must
not turn deterministic fixture coverage or context-growth ranges into a broad
autonomous-coding claim.
