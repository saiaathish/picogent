# Picogent v4 baseline

Status: reconstructed from the current checkout on 2026-08-25.

## Checkout

- Branch during reconstruction: `main`
- Exact v3 head: `a07943b31044049afb0142f39198244cd3c75218`
- Remote: `origin/main` at the same SHA
- Go: `1.26.6`
- Host: Apple M3 arm64 macOS
- Tracked files: 212
- Product surfaces: GUI local browser page, TUI, and headless `run`

The worktree already contained user-owned changes before v4 work:

- `.gitignore` adds `.gstack/`
- untracked `graphify-out/` contains a graphify report, graph, caches, and
  query artifacts

These paths are preserved and are not part of the v4 implementation.

## Fresh local gates

Observed on the exact v3 head before implementation:

| Check | Result |
| --- | --- |
| `go test ./...` | PASS |
| `go test -race ./internal/agent ./internal/gui ./internal/tui ./internal/taskstate ./internal/acceptance` | PASS |
| `go vet ./...` | PASS |
| `go build -o /tmp/picogent-v4-baseline ./cmd/picogent` | PASS |

The first parallel baseline run hit a Go module-cache rename warning while
other Go commands were running, but the build exited successfully and a later
targeted test run passed. Treat this as local tooling noise, not product
evidence.

The first v4 focused GUI run also exposed a test-isolation defect: the scoped
turn tests reused fixed session IDs under the default shared `PICOGENT_HOME`.
That suite failed on clean v3 `main` after 10-second waits, while the same
tests passed in `0.748s` with a private state directory. The tests now set a
per-test `PICOGENT_HOME`; this is test-environment hardening, not a v4 product
behavior change.

## Deterministic performance baseline

Commands followed `docs/V3-BENCHMARKS.md` with three repetitions. The current
head was measured on the same Apple M3 arm64 host.

| Operation | Observed range | Additional signal |
| --- | ---: | --- |
| Context manage, working set | 45.0–46.7 µs/op | 12 messages, 592 tokens, 105,067–105,069 B, 243 allocs |
| Context manage, context-heavy | 2.35–2.40 ms/op | 7 messages, 1,437 tokens, 486,407–487,430 B, 659–661 allocs |
| Repo-map inspect | 23.98–30.57 ms/op | 51,764–54,779 B, 288–290 allocs |
| Repo-map format | 2.818–2.865 µs/op | 188.83–191.98 MB/s, 2,370 B, 12 allocs |
| Session metadata list, 60 records | 8.53–9.29 ms/op | 183,200 B, 1,962 allocs |
| Session load | 178–319 µs/op | 3,368 B, 37 allocs |
| Verification plan | 1.937–1.963 µs/op | 864 B, 15 allocs |
| Verification evidence status | 3.439–3.461 µs/op | 1,792 B, 1 alloc |
| Scripted agent edit | 0.517–0.836 ms/op | 2 model calls, 113,328–116,096 B, 1,114–1,151 allocs |

These are deterministic local controls, not live-provider, browser, or model
quality claims.

## What v3 actually provides

- Compact durable task state: inferred intent, definition of done, changed-file
  sequence, verification status, bounded repair loop, and resumable sessions.
- Conditional goal persistence using text plus monotonic revision, preventing
  stale same-text completion from clearing a newer goal.
- On-demand bounded repository map with no index, watcher, or background
  process.
- Causal memory and optional self-evolution with bounded records.
- Safe/Fast permissions, workspace and symlink containment, MCP boundary, and
  checkpoint-backed `/undo`.
- GUI/TUI/headless runtime sharing the same agent core.
- Repair-diversity gate after exact repeated verification fingerprints.

## Current evidence gaps

The v3 benchmark matrix still marks these scenarios `UNRECORDED` or proxy-only:

- live-provider outcome quality
- vague beginner requests
- multi-file features and refactors
- rendered GUI journeys and UI improvement quality
- long-horizon work beyond deterministic context proxies
- real restart/steer behavior under changing requirements
- browser, external research, and provider failure behavior
- cross-platform runtime behavior beyond CI compile/test gates

`docs/V3-BENCHMARKS.md` names commit `ec88cdd` for its initial capture, which
does not match the current v3 head. Re-run v3 comparison fixtures before any
v4 performance claim.

## First v4 architectural decision

Reuse `internal/taskstate.Task` as the compact outcome boundary. Extend it with
bounded constraints, risks, unresolved uncertainty, and source-labelled
evidence rather than adding a second planner or project index. Keep the legacy
verification slice for compatibility; mirror fresh verification into the new
ledger and inject only the latest compact evidence into resumed task context.

The branch implementation must prove three properties before expansion:

1. evidence survives persistence and restart without unbounded growth;
2. failed or stale evidence cannot imply completion; and
3. context injection remains materially smaller than replaying raw tool output.

## Discovery wave status

The required 15-specialty read-only wave was requested, but the local
delegation service returned `agent thread limit reached` before handles/results
were returned. Findings in this document are host-agent reconstruction only;
the wave remains an explicit `UNAVAILABLE` gate for later retry.
