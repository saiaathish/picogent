# v4 long-horizon outcome evidence

Status: S-lane contract merged via PR #307; the provider-independent M-lane
fixture is delivered in PR #308 on `codex/v4-long-horizon-outcome-m`. This
document defines the measurement boundary; it does not claim long-horizon
product success, live-provider quality, or release readiness.

## Purpose

The existing long-session fixture measures persistence size and retained-record
bounds at several horizons. This contract adds the missing evidence vocabulary
for asking whether a durable outcome remains truthful as turns mutate,
verify, restart, steer, recover, and eventually stop.

The report is ephemeral test evidence. It does not add a planner, watcher,
index, daemon, or second durable task store.

## Report contract

The Go types live in `internal/benchmark/long_horizon_contract.go` and use
schema `picogent.v4.long-horizon-outcome.v1`.

| Field | Requirement |
| --- | --- |
| `source_head` | Full 40-character commit SHA for the code under measurement |
| `baseline_head` | Optional full commit SHA when a comparison is actually run |
| `host`, `go_version`, `command` | Reproduction metadata, bounded and required |
| `observations` | One bounded observation for each turn, in strict turn/revision order |
| `invariant_failures` | Bounded list of concrete lifecycle invariant failures |
| `unverified` | Bounded list of behavior the fixture could not observe |

Each observation uses a fixed event vocabulary: `plan`, `mutation`,
`verification`, `restart`, `steering`, `recovery`, and `stop`. It records
criterion completion, the current mutation sequence, the mutation sequence
covered by verification, evidence freshness, recovery state, the existing
Outcome Engine stop policy (`CONTINUE`, `PAUSE`, `RECHECK`, or `UNKNOWN`), and
the derived completion eligibility.

## Fail-closed invariants

- Turn numbers start at one and increase by one; durable turn revisions are
  strictly increasing.
- `current` evidence must cover the current mutation sequence exactly.
- `stale`, `missing`, and `unverified` evidence can never be completion proof.
- A pending or failed recovery can never authorize a stop.
- Completion eligibility requires complete criteria, current evidence, a
  completed or unnecessary recovery, and the existing `RECHECK` stop policy.
- A report that records invariant failures cannot also record eligible
  completion for an observation.

The validator checks report shape and internal consistency only. The
production taskstate, verification, permission, and recovery contracts remain
authoritative for real behavior.

## M-lane validation

`TestLongHorizonOutcome` drives the existing durable contracts through a
deterministic eight-turn scenario: admission and planning, two mutation and
verification cycles, intent steering, an active-turn fresh-process recovery,
and a final verification. The fixture persists task state before the child
process boundary, requires a fresh workspace observation before persisted
proof can authorize completion, and records the existing Outcome Engine stop
policy at each observation.

The observed local report is bounded to:

| Metric | Observation |
| --- | ---: |
| Logical turns | 8 |
| Useful-progress observations | 6 |
| Stale-proof observations | 3 |
| Eligible stops | 3 |
| Fresh-process reloads | 2 |
| Recovery events | 1 |
| Retained task turns / evidence entries | 8 / 8 |

Run the M-lane fixture and its package checks from the exact source head:

```sh
go test -v ./internal/benchmark -run '^TestLongHorizonOutcome$' -count=1
go test -race ./internal/benchmark -run '^TestLongHorizonOutcome$' -count=10
go test ./internal/benchmark -count=1
```

The test log emits `source_head`, host, Go version, command, per-turn
observations, and explicit `UNVERIFIED` fields. The source SHA is intentionally
read at runtime rather than copied into this document, so every report remains
attached to the exact tree it measured.

The fixture does not measure live-provider quality, arbitrary crash windows,
rendered GUI/TUI behavior, or release readiness. Its fresh-process boundary is
a deterministic child test process and should not be interpreted as broad
runtime chaos coverage.

## S-lane validation

Run the focused contract tests from the exact source head:

```sh
go test ./internal/benchmark -run 'TestLongHorizonReport|TestLongHorizonCompletion' -count=1
```

The medium lane may use this report to record a scripted multi-turn scenario.
It must retain direct observations and explicit `UNVERIFIED` gaps instead of
turning the fixture into a live-provider or cross-platform runtime claim.
