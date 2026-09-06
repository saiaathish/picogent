# V4 crash-safe project run-lock recovery

Status: the project run-lock S lane and the composed active-turn M lane are
complete on `main` through [PR #313](https://github.com/saiaathish/picogent/pull/313)
and [PR #315](https://github.com/saiaathish/picogent/pull/315). This report is
refreshed against the exact current `main` SHA
`38b45ff99af0221f4b5dcbe16d356f78ff2b71a9`. The refresh is documentation-only
and adds no runtime observation.

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

## S lane — abrupt-owner release

The fixture was implemented at test checkpoint `d2362be` and delivered in
[PR #313](https://github.com/saiaathish/picogent/pull/313) from source
`69f969364d7407ec4d1f8553a294a887cabf8812`. Its hosted CI run
[33575722242](https://github.com/saiaathish/picogent/actions/runs/33575722242)
passed security, Ubuntu, Windows, macOS, and release-evidence. The merge
commit is `f29d08a3fecef524679840ee74a9009e5b2ceddd`, and post-merge `main`
validation
[33576065143](https://github.com/saiaathish/picogent/actions/runs/33576065143)
passed all five gates at that exact merge SHA.

Focused command:

```text
go test ./internal/taskstate -run 'TestStoreRunLock(SerializesFreshProcess|ReleasedAfterAbruptOwnerDeath)$' -count=1 -v
```

The contention-control and abrupt-owner tests passed at the fixture
checkpoint. Readiness markers and bounded polling establish ownership and
handoff without relying on a fixed sleep. The result proves that a fresh
process is not stranded by an abruptly terminated owner of the existing
kernel-backed run lock.

## M lane — active-turn composition

The existing `internal/agent/long_horizon_test.go` process-kill fixture was
extended at test checkpoint `758b8bc` and delivered in
[PR #315](https://github.com/saiaathish/picogent/pull/315) from source
`2f02f3f5289d31a3b717422dc1f41bc8a134e476`. Its hosted CI run
[33576883024](https://github.com/saiaathish/picogent/actions/runs/33576883024)
passed security, Ubuntu, Windows, macOS, and release-evidence. The merge
commit is `e3a35e7fcf098d1f5bdee501f73ac1d15f42521b`, and post-merge `main`
validation
[33577312666](https://github.com/saiaathish/picogent/actions/runs/33577312666)
passed all five gates at that exact merge SHA.

Focused command:

```text
go test ./internal/agent -run '^TestLongHorizonResumeAfterProcessKill$' -count=1 -v
```

The killed worker holds the project run lock while it durably admits the
active turn. A fresh process then crosses the same lock boundary through
`SetTaskSession` and `Run`, recovers the interrupted `recover` turn with
`process_restart` metadata, retains the follow-up turn, and keeps the
completion projection fail-closed. This composes the S-lane handoff with an
existing durable recovery scenario; it does not create arbitrary crash-window
or product-quality evidence.

## Exact-main documentation checkpoint

This report is based on current `main`
`38b45ff99af0221f4b5dcbe16d356f78ff2b71a9`, the merge of
[PR #437](https://github.com/saiaathish/picogent/pull/437). Its exact-head
workflow
[33980834057](https://github.com/saiaathish/picogent/actions/runs/33980834057)
passed security, Ubuntu, Windows, macOS, and release-evidence.

That workflow validates the current tree, but this documentation checkpoint
does not rerun or broaden the historical S/M fixture observations. Their
claims remain bound to the source commits, merge commits, and hosted runs
recorded above.

## Remaining evidence boundary

No unfinished S/M project run-lock implementation lane remains on `main`.
The broader [#311](https://github.com/saiaathish/picogent/issues/311) scope
still does not prove:

- arbitrary crash timing inside every task or workspace write critical
  section;
- preservation against an uncooperative same-UID filesystem writer,
  symlink-swap, or other pathname/TOCTOU race;
- durable task completion or recovery by itself;
- rendered GUI/TUI behavior or rendered browser behavior;
- live-provider quality, multi-hour stability, release authorization, or
  overall v4 readiness.

These boundaries remain `UNVERIFIED` until directly observed with the
appropriate crash-window, adversarial, rendered, provider, or release
harness. This report does not claim that the current-main CI checkpoint
changes those limits.
