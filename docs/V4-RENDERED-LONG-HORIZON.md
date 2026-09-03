# v4 rendered long-horizon outcome contract

Status: size:S contract for [#366](https://github.com/saiaathish/picogent/issues/366),
under the long-horizon evidence parent
[#306](https://github.com/saiaathish/picogent/issues/306) and the broader
outcome contract [#246](https://github.com/saiaathish/picogent/issues/246).
The medium task-owned fixture is delivered through [#368](https://github.com/saiaathish/picogent/issues/368) / PR #369, and the large direct browser checkpoint is recorded in [#370](https://github.com/saiaathish/picogent/issues/370). Remaining evidence limits are explicit below.

## Boundary

The existing provider-independent long-horizon contract records authoritative
criterion, mutation, verification, recovery, and stop state. The rendered
contract adds only the small projection needed to compare that state with a
settled GUI observation. It does not add a planner, durable store, daemon,
watcher, provider, or user-facing workflow.

The schema is `picogent.v4.rendered-long-horizon.v1`, implemented by
`internal/gui/rendered_long_horizon_contract.go`.

## Required provenance

Every report must record:

| Field | Requirement |
| --- | --- |
| `source_head` | Full 40-character commit SHA for the tree under observation |
| `source_sha_verified` | Whether the supplied SHA matches the clean compiled source revision |
| `source_tree_modified` | Whether the source tree was dirty when the runtime was built |
| `host`, `runtime`, `command` | Bounded reproduction metadata |
| `browser_session`, `browser_tab` | Task-owned browser identifiers, without transcript or provider text |
| `observed_at_utc` | RFC3339 timestamp for the settled observation |

A dirty source cannot be marked verified. An unverified source must carry an
explicit entry in `unverified`; the contract never silently turns missing
provenance into a pass.

## Observation contract

Each observation contains one existing `benchmark.TurnObservation` plus the
rendered projection:

```json
{
  "outcome": {
    "turn": 2,
    "turn_revision": 2,
    "events": ["steering", "restart", "recovery"],
    "criteria_complete": false,
    "mutation_seq": 2,
    "verified_mutation_seq": 1,
    "evidence": "stale",
    "recovery": "complete",
    "stop": "CONTINUE",
    "completion_eligible": false
  },
  "rendered": {
    "task_present": true,
    "task_status": "working",
    "progress_visible": true,
    "completion_ready": false,
    "completion_marker": false,
    "changed_files": ["outcome.txt"]
  }
}
```

The benchmark validator remains authoritative for event vocabulary, strict
turn/revision ordering, evidence freshness, recovery state, and fail-closed
completion eligibility. The rendered validator adds these projection rules:

- a present task has a known task status and visible progress;
- a missing task cannot carry task details or visible progress;
- rendered readiness must equal authoritative completion eligibility;
- a rendered completion marker must equal rendered readiness; and
- changed-file names and all metadata remain bounded.

Therefore a mutation or steering event that leaves proof stale cannot render a
ready task, even if an earlier turn was eligible. A marker cannot appear for a
blocked, recovering, incomplete, stale, missing, or unverified observation.

## Size:S validation

Run from the exact source head:

```sh
go test ./internal/gui -run 'TestRenderedLongHorizon' -count=1
go test ./internal/gui -run 'TestRenderedLongHorizon' -count=10
```

The tests cover a valid authoritative/rendered pair, false completion after
steering, missing source-boundary disclosure, dirty-source rejection, unknown
rendered task state, and missing progress. They are deterministic in-process
contract tests; they do not claim a launched browser or a completed rendered
multi-turn run.

## Medium fixture lane

The task-owned medium fixture is implemented in
[#368](https://github.com/saiaathish/picogent/issues/368). It extends the
existing build-tagged rendered fixture with a deterministic multi-turn flow
over the normal GUI `/api/chat`, `/api/state`, permission, SSE, and reload
boundaries. Its integration test records bounded observations for mutation,
verification, steering, recovery, and stop eligibility, while keeping direct
browser DOM and live-provider behavior `UNVERIFIED`. Operator instructions
are in `docs/V4-RENDERED-LONG-HORIZON-FIXTURE.md`.

## Large direct evidence checkpoint

The direct observation record is in
`docs/V4-RENDERED-LONG-HORIZON-EVIDENCE.md` for [#370](https://github.com/saiaathish/picogent/issues/370).
It was captured against exact merge/current `main` SHA
`993258f4b97d196fd7c44cca78c235080fd062e9` using a clean VCS-stamped fixture
binary. Both seed and reload manifests verified the source SHA and reported a
clean source tree. The task-owned BrowserOS session directly observed Safe-mode
permission, mutation visibility, inconclusive verification, deterministic
workspace verification, steering invalidation, reload continuity, and the
post-reload fail-closed state.

This closes only the bounded direct-observation checkpoint. Browser screenshots
were captured inline by the task-owned session, but no persisted path was
exposed, so the screenshot path remains `UNRECORDED`. Live-provider quality,
cross-platform rendered behavior, arbitrary hostile writers, broader crash
windows, and release authorization remain `UNVERIFIED`.

## Remaining evidence limits

The medium and bounded large rendered lanes are now recorded, but live-provider
quality, unsupported cross-platform rendering, arbitrary hostile filesystem
writers, broader crash windows, release authorization, and v4 completion remain
`UNVERIFIED`.
