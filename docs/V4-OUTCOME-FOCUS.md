# V4 transient outcome focus

Picogent now has a small internal selector that connects the latest durable
task state to the next safe category of work. A fresh `project_health`
observation can enrich that category, but recovery, steering, mutation, and
verification transitions also rebuild the advisory from task state alone. It
answers only:

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

The agent gives the first model request and each relevant post-transition
request one bounded task-derived advisory. That view uses an explicitly
`UNKNOWN` health status unless a successful `project_health` call produced a
fresh observation after the latest durable mutation. If that read-only call
was co-batched with a successful write, its health-specific focus is discarded
and the next request receives the post-write task view instead.

The advisory is transient: it is not added to returned conversation history,
durable task state, permissions, or completion authorization. A fresh health
observation may guide exactly one subsequent model request.

This is intentionally not evidence invalidation. `repomap` metadata and Git
dirty paths cannot prove content equality, and task persistence has no
cross-process compare-and-swap contract. Durable evidence and completion
remain governed by the existing verifier and task-state change sequence.

## Verification and cost

Focused tests cover blocker and verification precedence, intent-fit ranking,
criterion fallback, malformed/hostile finding data, schema validation,
instruction bounds, durable mutation/recovery guidance, successful agent-loop
injection, and co-batched write freshness. The deterministic benchmark is:

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
