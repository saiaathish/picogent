# V4 trace recovery evidence

Status: deterministic local evidence recorded on 2026-08-31. The trace
crash-handoff test merged in PR #226 at exact `main` head
`597c7cb1a267686286b17cc180fd5958c6954a40`; the tested PR head was
`080c6e577ccfee8ddb1ea81c35046fda6a9c14ea`.

## Invariant under test

`internal/trace/process_reconnect_test.go` starts a real child process that
acquires the trace lock and publishes a readiness marker. The parent kills
that child without allowing a graceful unlock, starts a fresh process, and
requires it to append successfully. The parent then reads the trace and
requires the pre-crash and post-crash events to have contiguous sequence
numbers.

This exercises the kernel-backed lock release that occurs when a process dies,
the fresh process's `Open`/`Append` path, and sequence recovery from the durable
JSONL tail. It does not rely on an in-process mutex to simulate a process
boundary.

## Repeated evidence

- 100 repetitions of the killed-lock-holder handoff passed.
- 20 repetitions of the combined trace process tests passed.
- 10 race repetitions of the trace process tests passed.
- The complete `go test ./...` suite passed locally.
- PR #226 passed the hosted Ubuntu, Windows, macOS, security, and
  `release-evidence` gates.

The hosted matrix is cross-platform test evidence for the committed code. The
repeated crash-handoff measurements above were local deterministic stress
runs; they are not a cross-platform runtime budget.

## Boundary of the claim

The proof is intentionally narrow. It does not establish session restart or
resume semantics, GUI/TUI/headless reconnect behavior, safety against an
uncooperative same-UID filesystem writer, symlink-swap or other TOCTOU races,
live-provider quality, or rendered runtime behavior. Those remain separate
release-audit items and must remain `UNVERIFIED` until directly observed.
