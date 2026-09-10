# Adaptive depth measurement

Issue #591 records the first measurement lane for the adaptive task-depth
contract. The experiment is intentionally benchmark-only: it does not invoke a
model, tool, planner, specialist, or workspace mutation.

## Reproduction

```shell
GOMAXPROCS=2 go test -p 1 ./internal/benchmark -run 'Test(MeasureAdaptiveDepth|AdaptiveDepthMeasurement)' -count=1 -v
```

The run uses the existing 20-scenario outcome-quality catalog and the real
`outcome.Build` path. The fixed comparator assigns one standard quality loop
to every case. The adaptive side uses the versioned `TaskDepthProfile` and
converts its categorical budget into bounded experiment work units only for
comparison; those units are not production token or latency promises.

## Observed proxy result

| observation | result |
| --- | ---: |
| catalog scenarios | 20 |
| focused profiles | 4 |
| deep profiles | 14 |
| broad profiles | 2 |
| fixed-standard budget under-adequate cases | 13 |
| adaptive budget-adequate proxy cases | 20 |
| fixed comparator work units | 13 |
| maximum adaptive work units | 25 |
| configured maximum work units | 32 |

The adaptive proxy distinguishes higher-cost categories while staying below
the explicit work cap. This is routing-budget evidence only.

## What remains unproven

`quality_impact` is deliberately `inconclusive`. The current profile is
advisory, so this lane does not isolate two executions whose only difference
is fixed versus adaptive routing. No claim about correctness, user questions,
tokens, model calls, tool calls, latency, or real outcome quality follows from
the proxy result.

A later quality lane must use the same fixture, executor, source, environment,
and bounded policy for both routes, change only the routing decision, and
record explicit pass/fail/inconclusive evidence before adaptive depth can be
used as a production routing gate.

## Quality-isolated lane (#593)

Issue #593 adds that later lane with the same deterministic fixture, scripted
agent executor, target provenance, input digest, and metric policy for both
routes. The only route difference is the bounded `MaxTurns` value derived from
the fixed one-loop comparator or the adaptive profile. Each catalog scenario is
run twice, in stable scenario/repetition order.

```shell
GOMAXPROCS=2 go test -p 1 ./internal/benchmark -run 'Test(MeasureAdaptiveDepthQuality|AdaptiveDepthQuality)' -count=1 -v
```

Observed deterministic result:

| observation | result |
| --- | ---: |
| catalog scenarios | 20 |
| repeated paired observations | 40 |
| focused/deep/broad observations | 8 / 28 / 4 |
| fixed-route budget exhaustions | 26 |
| adaptive-route budget exhaustions | 0 |
| adaptive quality deltas marked pass | 26 |

The report is serialized and validated, including stable catalog order,
repetition order, input digests, target provenance, metric bounds, route caps,
and the generalization boundary. This is evidence that the bounded scripted
fixture distinguishes the two route limits. It is not evidence of improved
quality for live providers or arbitrary repositories, and it does not promote
adaptive depth from advisory guidance to production authority.
