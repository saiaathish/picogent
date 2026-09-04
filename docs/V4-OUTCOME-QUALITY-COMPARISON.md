# V4 exact-head outcome-quality comparison

Status: the S reproducibility checkpoint for [#422](https://github.com/saiaathishkarthik/picogent/issues/422) and [#246](https://github.com/saiaathishkarthik/picogent/issues/246).

The outcome-quality contract and scripted executor are already defined in
[`V4-OUTCOME-QUALITY-BENCHMARK.md`](V4-OUTCOME-QUALITY-BENCHMARK.md). This
document closes one narrower gap before a comparative run: a declared source
SHA must correspond to the clean source tree that will execute the target.

## S-lane preflight

`ValidateOutcomeQualitySourcePair` accepts one
`OutcomeQualitySourceBinding` for each target. It requires:

- an absolute, resolvable workspace directory;
- distinct baseline and candidate workspaces;
- distinct full source commit IDs;
- matching host, Go, and runner metadata;
- committed `HEAD` equal to each target's declared source SHA; and
- a clean worktree, including untracked files.

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

## Required M/L follow-up

The next execution lane must build or launch each target from the validated
workspace without shell interpolation, send the same bounded input and policy
to both targets, cap child output and runtime, and validate the returned
observations with `OutcomeQualityReport.Validate()`. A complete report still
requires the fixed 20-scenario catalog, at least two repetitions, deterministic
ordering, current verification evidence, and explicit failure or
`UNVERIFIED` reasons.

Only after that report is complete may the L lane compare scenario/category
deltas or update the scorecard. Live-provider quality, rendered behavior,
arbitrary repository success, release authorization, and overall v4 readiness
remain outside this evidence boundary.
