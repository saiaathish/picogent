# v4 retention-value contract

Status: L lane (`#406`) candidate, based on the merged M lane (`#405`, PR
`#416`) at `main` `5a45b46`.

The contract is versioned as `picogent.retention-value.v1`. It defines a
provider-independent way to assess and rank complete history units. Durable
session saves use the contract for bounded history selection, and the live
context-compaction boundary now uses the same bounded structural selection.

## Boundary

`internal/retention` accepts a structural projection, not an
`llm.Message`. A caller supplies only:

- a bounded list of roles and tool-call identifiers needed to validate pairing;
- a bounded current-turn flag and numeric position coordinates;
- typed, allowlisted outcome, verification, recovery, and error markers.

The projection has no content, parts, names, commands, tool arguments, tool
results, repository paths, or model-output fields. Assessments contain only
the version, closed enums, booleans, bounded scores, and bounded numeric
coordinates. Tool-call identifiers are used transiently for pairing and are
never copied into an assessment, rank result, or selection result.

The caller remains responsible for deriving markers from authoritative state.
The contract does not interpret arbitrary transcript text, model narration, or
repository content as policy.

## Allowlisted vocabulary

Every marker family is a closed string type. The Go zero value is accepted as
unmarked and normalized to `unmarked` in an eligible assessment.

| Family | Allowlisted values |
| --- | --- |
| outcome | `unmarked`, `completed`, `changed`, `blocked`, `failed` |
| verification | `unmarked`, `passed`, `failed`, `inconclusive`, `skipped` |
| recovery | `unmarked`, `resumed`, `restored`, `repaired` |
| error | `unmarked`, `observed`, `resolved`, `unresolved` |

Unknown marker values fail closed as `UNVERIFIED` and are not echoed. There is
no free-form marker or reason field.

## Structural eligibility

`Assess` applies the following rules in order:

1. An empty unit, overlarge unit, invalid coordinate, unknown role, invalid
   identifier, duplicate call identifier, or malformed structural field is
   `UNVERIFIED`.
2. A system message is `INELIGIBLE`; it is recognized so an internal prompt is
   never accidentally treated as an unknown user unit.
3. A tool result without a matching pending assistant call is
   `INELIGIBLE` with `orphan-tool-result`.
4. An assistant call whose result is missing, or a new user/assistant message
   before its results arrive, is `INELIGIBLE` with
   `incomplete-tool-pair`.
5. A non-tool unit or a unit whose assistant calls all have exactly one
   matching result is `ELIGIBLE`.

The pair validator accepts matching results in any order but requires every
call identifier exactly once. It never accepts an orphan or partial pair.

## Deterministic ranking

`Rank` is pure and does not mutate its input. It sorts by these keys, in order:

1. eligibility (`ELIGIBLE` before `INELIGIBLE` before `UNVERIFIED`);
2. descending bounded score;
3. current-turn priority (`true` before `false`);
4. descending position, where a larger position is newer;
5. ascending original index;
6. ascending input index as a final tie-break when callers provide duplicate
   coordinates.

When a unit is rejected but its coordinates are individually within the
bounded range, `Assess` preserves `CurrentTurn`, `Position`, and
`OriginalIndex` so rejected candidates still have the documented deterministic
ordering. Invalid coordinates remain safe zero defaults and produce
`UNVERIFIED/invalid-position`.

The score is capped at 100. Its bounded contributions are:

- role: user 24, assistant 16;
- complete tool pair: 12;
- completed/changed outcome: 20, blocked/failed outcome: 14;
- passed/failed verification: 18, inconclusive verification: 12;
- any recovery marker: 14;
- observed error: 16, resolved error: 12, unresolved error: 20.

The existing verifier's `SKIPPED` status maps to the explicit `skipped`
verification marker. A skipped verification is not a pass: it keeps the unit
eligible when the rest of its structure is valid but contributes zero score,
like `unmarked`.

The score is intentionally a small deterministic heuristic, not a model
judgment or completion authority. Durable task state and evidence remain the
authoritative source for outcome and verification claims.

`Select` takes the highest-ranked eligible units and then sorts just that
subset by ascending original index. This preserves transcript order after
selection. The durable session path uses the same `Rank` ordering while
applying its byte/message capacity and preserving the newest user request and
newest complete turn as anchors.

## Live compaction integration

The L lane applies the contract inside `internal/ctxmgr.Manage` through
`ValueAwareWindow`. The live projection supplies only message roles and
tool-call identifiers, so outcome, verification, recovery, and error markers
remain `unmarked` at this boundary. The selection therefore improves the
structural value of the live window without treating transcript text or model
narration as a value signal.

The live window:

- preserves the system prompt, newest user request, and newest complete turn;
- selects older complete units by deterministic structural rank and restores
  their original transcript order;
- never emits an incomplete or orphaned tool exchange;
- bounds the candidate projection to `MaxUnits` before ranking; and
- falls back to the existing recency tail only when no usable candidate set is
  available.

The implementation checkpoint `af3ca8c` was measured on an Apple M3 arm64 Mac
with Go `1.26.6`. In the focused 10-message live-window fixture, the
value-aware path retained the historical complete target in every run while
the recency-only control did not:

| Window | Time range | B/op | allocs/op | target coverage |
| --- | ---: | ---: | ---: | ---: |
| value-aware | 23.3–30.6 µs | 65,632 | 226 | 1.000 |
| recency-only | 0.296–0.378 µs | 2,176 | 2 | 0.000 |

This is roughly a 60–100x latency increase and a materially larger allocation
footprint for this small control. The 96-turn long-horizon stress fixture
reduced 384 raw messages to 18 managed messages and retained the historical
complete `read-088` exchange while the recency control did not. Its full
end-to-end benchmark measured 2.51–2.91 s/op, 214.7–216.1 MiB/op, and
511,895–512,187 allocations/op; those figures include the persistence and
reload workload and are not a causal before/after allocation comparison.

### Conditional ship/delete decision

Keep the L integration as a conditional ship candidate: the direct fixture
benefit is reproducible, the selection remains bounded, and no new user-facing
workflow or authority boundary was introduced. The cost is high relative to a
recency tail, and this boundary currently has no authoritative task-value
markers. If a production-shaped measurement shows that this overhead is
user-visible without improving useful context beyond structural tool-pair
preservation, delete the live integration and retain the provider-independent
S/M contract.

## Bounds

The contract rejects a ranking request above 256 units. Each unit is limited to
16 structural messages, each assistant message to 8 call identifiers, each
identifier to 128 bytes of valid UTF-8, and each coordinate to the range
`0..2^20`. Selection limits are also bounded to `0..256`.

These bounds apply before a value can affect an assessment. Invalid values are
never clamped into an apparently useful result.

## Verification and lane boundary

Focused validation for this lane:

```text
go test ./internal/retention
go test -race ./internal/retention
go vet ./internal/retention
```

The tests cover deterministic ranking, selection-order restoration, hard
bounds, complete and incomplete tool pairs, malformed/unknown fallback,
allowlisting, hostile instruction-like values, input immutability, and raw
text leakage.

The M integration (`#405`, merged through PR `#416`) was measured against the
pre-integration recency-only control with a deterministic fixture. Its focused
comparison records retained-value coverage, representative unit counts,
latency, and allocations. The L fixture now demonstrates the same bounded
structural benefit at live compaction and across a 96-turn local long-horizon
run. All of these numbers are local implementation measurements, not
live-provider quality evidence. Rendered behavior, cross-platform runtime
behavior beyond hosted CI, broad autonomous-coding quality, and release
authorization remain unverified.
