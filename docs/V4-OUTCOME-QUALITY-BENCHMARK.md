# V4 outcome-quality benchmark contract

Status: the S-lane measurement contract for issue #323 and parent issue #246.
This document defines what a future runner may measure; it is not a live-
provider quality result or a release authorization.

## Purpose

Picogent already has deterministic performance benchmarks and a provider-
independent long-horizon integrity fixture. Those answer whether selected
operations are bounded and whether durable state remains truthful. They do not
answer whether v3 and v4 complete the same representative tasks with the same
inputs and comparable work.

The report contract is `picogent.v4.outcome-quality.v1` and is implemented in
`internal/benchmark/outcome_quality_contract.go`. It is ephemeral evidence; it
does not replace `taskstate`, verification, permission, undo, or the Outcome
Engine completion authority.

## Stable scenario matrix

`DefaultOutcomeQualityScenarios` contains 20 deterministic definitions. Each
definition has a stable ID, brief category, task shape, and seed:

| Category | Scenario shapes |
| --- | --- |
| Beginner | vague feature, broken app, setup problem |
| Standard development | bug, feature, refactor, tests |
| Advanced | migration, architecture, performance, security |
| Product | UI polish, onboarding, launch readiness |
| Robustness | resume, cancel, steer, undo, conflicting edits |
| Long horizon | multi-stage project improvement |

The report records one input SHA-256 digest per definition. The digest binds
the exact fixture or prompt input used by both variants; it is not inferred
from a scenario label or a transcript.

## Reproducibility contract

Every report records:

- full baseline and candidate commit SHAs;
- host, Go version, and benchmark tool version for both targets;
- one shared timeout and work budget;
- at least two repetitions;
- one command and the complete stable scenario matrix;
- observations ordered by scenario, baseline then candidate, and repetition;
- explicit `invariant_failures` and `unverified` reasons when coverage or
  interpretation is incomplete.

Validation rejects different host/tool metadata, equal source heads, missing
input digests, one-run reports, partial reports marked complete, mixed source
heads, duplicate or out-of-order observations, unknown taxonomy values, and
unbounded metadata or counters.

The shared policy prevents the candidate from receiving a different timeout,
token budget, model-call budget, tool-call budget, or turn budget. A future
runner must still ensure that the environment and fixture contents are actually
the same; the report can only reject mismatched recorded metadata.

## Metrics

Each observation records:

- `outcome_success` and `correctness` as `pass`, `fail`, `inconclusive`, or
  `unverified`;
- user questions, tokens, model calls, tool calls, latency, changed lines,
  unnecessary changes, repair count, and context growth;
- `verification_quality` as `pass`, `fail`, `inconclusive`, `skipped`, or
  `unverified`;
- criterion-bound `evidence` using the existing `current`, `stale`, `missing`,
  and `unverified` vocabulary.

The contract is fail-closed: a passing verification requires current evidence,
passing correctness requires current passing verification, and passing outcome
success requires both. Recorded invariant failures also forbid a passing
outcome observation.

## Validation command

Run the contract checks with:

```sh
go test ./internal/benchmark -run '^TestOutcomeQuality' -count=1
```

The S lane is intentionally provider-independent and does not invoke a model,
network, browser, or real upstream repository. The later M lane may reuse the
existing deterministic provider stubs and taskstate/verification seams, but it
must not create a second planner, durable store, report authority, daemon,
watcher, or user-facing workflow.

## Evidence limits

This contract alone does not establish:

- live-provider completion quality, model ranking, or product SLAs;
- success on arbitrary real repositories or rendered GUI/TUI journeys;
- restart, cancel, steer, or undo behavior beyond direct observations supplied
  by a later runner;
- blind-review agreement or a meaningful v4 improvement;
- cross-platform runtime performance, security readiness, an SBOM, a signed
  production binary, or release authorization.

Those claims remain `UNVERIFIED` until their own focused evidence exists.
