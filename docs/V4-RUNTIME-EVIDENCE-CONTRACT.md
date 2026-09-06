# v4 runtime-evidence contract

This contract is the S lane under [#453](https://github.com/saiaathish/picogent/issues/453)
and [#460](https://github.com/saiaathish/picogent/issues/460). It defines the
smallest shared record used by later live-provider, rendered-platform,
hostile-runtime, recovery/undo, and release-authorization lanes. It is an
evidence boundary, not a release approval.

## Record requirements

Each `picogent.runtime-evidence.v1` record requires:

- a bounded claim and one category: `live_provider`, `rendered_platform`,
  `hostile_runtime`, `recovery_undo`, or `release_authorization`;
- one verdict: `PASS`, `FAIL`, `INCONCLUSIVE`, or `UNVERIFIED`;
- repository identity, a full source commit SHA, RFC3339 observation time,
  and OS/architecture/runtime environment;
- a bounded setup description;
- at least one reference to retained proof or a task-owned observation; and
- at least one explicit limitation.

The Go validator rejects missing fields, unsupported verdicts/categories,
short or malformed commit IDs, malformed timestamps, invalid SHA-256
references, unknown JSON fields, multiple JSON values, and oversized records.
It does not normalize an invalid record into `UNVERIFIED` or `PASS`.

## Evidence boundaries

Repository tests and deterministic fixtures may support a record only for the
behavior they directly observe. A local stub is not live-provider evidence; a
rendered fixture is not arbitrary-user or unsupported-platform evidence; a
cross-compiled binary is not native runtime evidence; and green hosted CI is
not release authorization. A missing screenshot, unavailable provider, or
unexercised hostile case must remain explicit in `references` and
`limitations`, with the verdict set to `INCONCLUSIVE` or `UNVERIFIED`.

The later M lane (#461) owns matrix retention and external artifact layout.
The L lane (#453) owns direct task-owned execution of live, rendered, hostile,
recovery, and release-boundary observations. Neither lane may close the
release-readiness parent from this contract alone.

## Focused validation

```sh
go test ./internal/verify -run '^Test(RuntimeEvidence|ValidateRuntimeEvidence)' -count=1
```

