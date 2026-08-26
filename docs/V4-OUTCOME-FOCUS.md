# V4 transient outcome focus

Picogent now has a small internal selector that connects an active durable
task to one fresh `project_health` observation. It answers only:

> What is the next safe category of work for this turn?

The selector lives in `internal/outcome` and remains transient. It does not
write state, cache a repository diagnosis, watch the filesystem, or claim that
an outcome is complete.

## Selection contract

The precedence is deliberately conservative:

1. an explicit durable blocker wins;
2. an unverified mutation wins and requests the narrowest relevant check (or a
   broader check when the changed-file list was capped);
3. a known project-health finding is selected using its existing bounded
   priority plus a small task-intent fit tie-breaker;
4. the current incomplete durable criterion is selected;
5. the selector falls back to inspection with `UNVERIFIED` evidence.

The selector accepts only known project-health finding IDs. Their next-action
categories are fixed in code, so manifest or tool text cannot become a new
system instruction. The internal prompt does not expose priority numbers or a
new user-facing mode.

## Freshness boundary

The agent adds this guidance only after a successful `project_health` tool
call. If that read-only call was co-batched with a successful write, the
guidance is discarded because the snapshot may predate the mutation. The next
model round can request a new snapshot when it needs one.

This is intentionally not evidence invalidation. `repomap` metadata and Git
dirty paths cannot prove content equality, and task persistence has no
cross-process compare-and-swap contract. Durable evidence and completion
remain governed by the existing verifier and task-state change sequence.

## Verification and cost

Focused tests cover blocker and verification precedence, intent-fit ranking,
criterion fallback, malformed/hostile finding data, schema validation,
instruction bounds, successful agent-loop injection, and stale co-batched
writes. The deterministic benchmark is:

```sh
go test ./internal/benchmark -run '^$' \
  -bench '^BenchmarkOutcomeFocus$' -benchtime=100ms -benchmem -count=3
```

The benchmark measures local selection and prompt construction only. It is not
evidence of live-provider quality, broad outcome completion, or release
readiness. On this Apple M3 arm64 macOS host with Go `1.26.6`, three runs
measured `395.3–407.8 ns/op`, `1,424 B/op`, and `6 allocs/op`.

## Known limits

- The selector cannot determine whether a repository file changed when the
  captured metadata stayed the same.
- It does not yet attach criterion-level evidence or predict change impact.
- A selected category is advisory; permissions, live tools, verification, and
  explicit user scope remain authoritative.
