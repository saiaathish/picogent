# v4 contradiction evidence contract

This document defines the small contradiction signal used by the v4 Outcome
Engine work. The signal is derived from the existing task evidence ledger; it
is not a second ledger, planner, completion gate, or user-facing agent.

## Why it exists

Picogent already records bounded evidence with a proof kind, optional criterion
binding, producer origin, and workspace change generation. It also rejects a
single verification result that claims PASS while reporting failed or
truncated evidence. That protects one result, but it does not describe
disagreement between two otherwise comparable observations.

The contradiction detector makes that disagreement explicit so a later
Outcome Engine lane can choose a conservative diagnostic or recheck route.
This S lane does not change routing or completion behavior.

## Comparable boundary

Evidence records are comparable only when all three values match:

1. canonical evidence kind (`verification`, `research`, `measurement`,
   `visual`, `tests`, `approval`, or `inspection`);
2. criterion index, or `-1` for aggregate evidence; and
3. the exact `ChangeSeq`.

Stale generations are ignored. A positive status is `PASS`, `APPROVED`, or
`CONFIRMED`. A negative status is `FAIL`, `INCONCLUSIVE`, or `SKIPPED`.
One of each in the same current boundary produces a signal.

## Derived schema

The JSON schema identifier is `picogent.outcome-contradiction.v1`.

```json
{
  "schema": "picogent.outcome-contradiction.v1",
  "state": "CONFIRMED",
  "signals": [
    {
      "scope": "requirement",
      "kind": "tests",
      "criterion_index": -1,
      "change_seq": 3,
      "positive_status": "PASS",
      "negative_status": "FAIL",
      "positive_origin": "test_runner",
      "negative_origin": "test_runner",
      "state": "CONFIRMED"
    }
  ]
}
```

`scope` is `criterion` for criterion-bound evidence, `requirement` for the
quality kinds, and `aggregate` for other unbound evidence. The report is
stable-ordered by kind, criterion index, and change generation.

## Trust and supersession

Both records must still carry runtime trust and a producer origin allowed for
their kind before the signal is `CONFIRMED`. Caller-supplied evidence and
evidence reloaded from JSON can be observed, but its runtime trust is absent;
those signals are `ADVISORY` and cannot satisfy completion or become an agent
instruction. Origins are reduced to known producer labels or `untrusted`.

Existing intent-change, workspace-restoration, and stale-verification paths
emit fixed invalidation provenance. Those records supersede the older PASS
for their boundary; they are not contradictory observations. The provenance
is recognized from the existing source/origin/reference contract and remains
recognizable after reload, while reload still removes runtime trust.

## Bounds and safety

- at most eight signals are emitted;
- truncation is explicit through `signals_truncated`;
- formatted JSON is capped at 4 KiB;
- summaries, commands, references, timestamps, repository text, and model
  output never enter the snapshot or report;
- the report is read-only and derived on demand;
- taskstate completion remains the only completion/retirement authority.

The signal is evidence of disagreement, not proof of which observation is
correct. It does not claim semantic fact extraction, live-provider quality,
rendered behavior, release readiness, or protection against arbitrary hostile
filesystem writers. Those boundaries remain `UNVERIFIED` until separately
observed.
