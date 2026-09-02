# V4 crash-safe project run-lock recovery

Status: the S-lane fixture is implemented at test checkpoint `d2362be` on
2026-09-02. The final PR source, merge, and post-merge main SHAs belong in the
issue and PR evidence records.

## Invariant under test

`internal/taskstate/store_test.go` starts a real child process that acquires
the existing `taskstate.Store.AcquireRunLock` and publishes a readiness marker.
The parent terminates that process without allowing its release callback to
run, waits for the process to exit, and starts a second fresh process against
the same lock directory. The second process must acquire and release the lock
within the existing bounded marker-polling harness.

This exercises the kernel-backed process-death release behavior at the project
run-lock boundary. It does not add a second lock or alter the production
locking implementation.

## Command and observed result

```text
go test ./internal/taskstate -run 'TestStoreRunLock(SerializesFreshProcess|ReleasedAfterAbruptOwnerDeath)$' -count=1 -v
```

The contention control and abrupt-owner test both passed locally on the
`d2362be` source checkpoint. The test uses readiness markers and a bounded
poll, rather than assuming that a fixed sleep establishes process ownership.

## M-lane composition

The existing `internal/agent/long_horizon_test.go` process-kill fixture was
extended at test checkpoint `758b8bc` so the killed worker holds the project
run lock while it durably admits the active turn. After the parent waits for
that worker to exit, the existing fresh-process `SetTaskSession` and follow-up
`Run` path must acquire the same lock before recovering the task. The fixture
then verifies the interrupted `recover` turn, `process_restart` metadata,
retained follow-up turn, and fail-closed completion projection.

```text
go test ./internal/agent -run '^TestLongHorizonResumeAfterProcessKill$' -count=1 -v
```

The integrated test passed locally in `0.12s`. This composes the S-lane lock
handoff with an existing durable recovery scenario; it does not turn either
fixture into arbitrary crash-window or product-quality evidence.

## Evidence boundary

This S lane proves only that a fresh process is not stranded by an abruptly
terminated owner of the existing kernel-backed run lock. It does not prove:

- arbitrary crash timing inside every task or workspace write critical
  section;
- preservation against an uncooperative same-UID filesystem writer,
  symlink-swap, or other pathname/TOCTOU race;
- durable task completion or recovery by itself;
- rendered GUI/TUI behavior, live-provider quality, or release readiness.

The conditional M lane may compose this handoff with the existing active-turn
recovery fixture. The conditional L lane may project only direct M-lane
observations through existing surfaces; unsupported behavior remains
`UNVERIFIED`.
