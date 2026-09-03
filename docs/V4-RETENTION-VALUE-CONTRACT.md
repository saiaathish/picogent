# v4 retention-value contract

Status: S lane (`#404`), contract and tests only.

The contract is versioned as `picogent.retention-value.v1`. It defines a
provider-independent way to assess and rank complete history units without
changing Picogent's current session eviction or live-compaction behavior.

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
selection. It is an unconnected contract helper in the S lane; it does not
evict session history.

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

This lane does not change `internal/session` or `internal/ctxmgr`. The M lane
(`#405`) may integrate the contract into durable retention only after this
contract is merged and a reproducible comparison against the current
recency-only control demonstrates useful-context benefit. Live compaction and
long-horizon evaluation remain conditional on M evidence in the L lane
(`#406`).
