# V4 exact-head outcome-quality comparison

Status: the prior exact source-pair matrix for [#422](https://github.com/saiaathish/picogent/issues/422) and [#246](https://github.com/saiaathishkarthik/picogent/issues/246) remains preserved in the [2026-09-04 bounded report](V4-OUTCOME-QUALITY-REPORT-2026-09-04.json). The refreshed run for [#435](https://github.com/saiaathish/picogent/issues/435) is recorded in the [2026-09-05 bounded report](V4-OUTCOME-QUALITY-REPORT-2026-09-05.json) and remains `INCONCLUSIVE` because the v3 baseline still lacks comparable structured telemetry.

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

The refreshed matrix is recorded below. It does not establish live-provider
quality, rendered behavior, arbitrary repository success, release
authorization, or overall v4 readiness. Those claims remain outside this
evidence boundary.

## Exact matrix result

The reviewed run used the fixed 20-scenario catalog, baseline-before-candidate
ordering, and two repetitions. It captured all 80 expected observations and
persisted a structurally valid report before the detailed evidence assertions
ran.

The complete per-observation JSON is committed as
[`V4-OUTCOME-QUALITY-REPORT-2026-09-05.json`](V4-OUTCOME-QUALITY-REPORT-2026-09-05.json).
It is a local `darwin/arm64` opt-in execution artifact. PR #436 fixed the
candidate full-fixture proof binding, and its post-merge hosted run
`33972008573` passed Ubuntu, macOS, Windows, security, and release-evidence
checks; those checks validate the source fix and do not claim to have rerun
this opt-in 80-observation matrix.

| Field | Recorded evidence |
| --- | --- |
| Baseline source | `a07943b31044049afb0142f39198244cd3c75218` |
| Candidate source | `e7160234e7a6a3c3efe8959cf9f9b56cc4c1f87f` |
| Matrix test-anchor head | `48eaf949a5e2cf3cf4e72e250305e97c3ffa5854` |
| Merge/current `main` at the run | `e7160234e7a6a3c3efe8959cf9f9b56cc4c1f87f` |
| Host/toolchain | `darwin/arm64`, `go1.26.6` |
| Runner | `picogent-outcome-quality-runner-v1` |
| Shared policy | 2 repetitions, 30-second observation timeout, 32 maximum turns |
| Coverage | 20 scenarios × 2 variants × 2 repetitions = 80/80 observations |
| Report status | `inconclusive` |

The recorded result is not a v4 quality win or regression claim:

| Variant | Observations | Observed boundary |
| --- | ---: | --- |
| v3 baseline | 40 | All remain `INCONCLUSIVE` because the exact v3 source does not expose structured repair-count/context-growth telemetry; 2 `advanced-architecture` observations also recorded a reproducible fixture write failure. |
| v4 candidate | 40 | All passed the deterministic fixture with current verification, including the required full three-file capture. |
| Comparison | 80 | No v3/v4 quality delta is claimable because the baseline telemetry boundary remains incomplete; candidate fixture proof is now complete. |

The catalog contains the following scenario counts. Counts are coverage
counts, not successful outcomes; every row remains `INCONCLUSIVE` for
comparison purposes.

| Category | Scenarios | Observations | Interpretation |
| --- | ---: | ---: | --- |
| Beginner | 3 | 12 | No comparable pass/fail delta recorded. |
| Standard development | 4 | 16 | No comparable pass/fail delta recorded. |
| Advanced | 4 | 16 | No comparable pass/fail delta; two baseline observations in `advanced-architecture` had fixture-write failures. |
| Product | 3 | 12 | No comparable pass/fail delta recorded. |
| Robustness | 5 | 20 | No comparable pass/fail delta recorded. |
| Long horizon | 1 | 4 | No comparable pass/fail delta recorded. |

## Decision and next evidence boundary

This is a complete observation-count and provenance checkpoint, not a
quality-improvement result. The candidate full-fixture proof gap is closed for
this deterministic lane, but a future comparison must add explicit compatible
proof for the v3 telemetry boundary and resolve or preserve the two baseline
fixture limitations. It must not turn deterministic fixture coverage into a
broad autonomous-coding claim.
