# Picogent v4 discovery scorecard

Status: refreshed on 2026-08-26 against exact `main` head
`745846d3b0399e58f14c8dbd6b81e66424218688`.

The required 15-specialty Wave A audit was run in bounded read-only batches on
2026-08-25; the findings below are carried forward and reconciled with the
landed slices since that audit.
Every specialist verified the same head and left the pre-existing worktree
changes untouched: modified `.gitignore`, untracked `graphify-out/`, and
untracked `picogent-go-tmp-umask`.

Ratings describe the current code, not an unverified live-provider or release
claim. `UNVERIFIED` means that source inspection or a deterministic test was
not enough to establish real runtime behavior.

## Specialty findings

| Specialty | Excellent | Adequate | Brittle | Wasteful | Highest-leverage next improvement |
| --- | --- | --- | --- | --- | --- |
| Architecture | Tiny local-first, single-agent boundary; bounded state | Compact `taskstate` model | Lifecycle and persistence semantics are distributed across `agent`, `goal`, `verify`, and the surfaces | Separate GUI side-chat path and repeated surface orchestration | Add one internal outcome/turn contract around the existing task state; do not add a second planner or index |
| Agent reasoning | Evidence-gated completion and stale-goal protection | Bounded repair loop and durable context | Keyword intent inference; repair diversity is prompt advice rather than route enforcement | Repeated admission/inference logic in GUI, TUI, and headless | Record intent revision, hypothesis/route, evidence, and stop reason in one shared contract |
| Intent/outcome | Monotonic goal revisions, tombstones, atomic clear | Permission boundary and bounded criteria | Template-only definition of done; ambiguity, negation, and conflicting goals are not structured | Duplicate `Steps`/`DefinitionOfDone` and `Verification`/`Evidence` representations | Make criteria and criterion evidence authoritative before expanding outcome features |
| Memory | Bounded task and session records with save-before-publish | Causal learning remains small and advisory | FIFO retention is not value-aware; cross-process task writes are last-writer-wins | Transcript-shaped session storage duplicates structured task/evidence state | Make retention value-aware after measuring restart and compaction behavior |
| Context efficiency | Pair-safe compaction; 8,192-character durable context; failure signals retained | Deterministic stale-output reduction | Aggregate summarization input and token estimates are not hard-bounded/calibrated; lexical priority misses structured/non-English failures | Repeated stale skeletonization/deduplication passes | Add an aggregate `Manage`/`Summarize` budget and measure bytes, tokens, allocations, and latency |
| Repo intelligence | On-demand deterministic map with no daemon/index/watcher | Bounded search and map output | Short-head/dirty status omits path provenance; nested roots and fallback semantics drift | Unused `Generate`/`Build` aliases and duplicate phrase/default command tables | Add a bounded provenance-bearing repo snapshot refreshed at admission and after mutation |
| Verification | Explicit `PASS`/`FAIL`/`INCONCLUSIVE`/`SKIPPED`; targeted-to-broader stages | Changed-file cap forces broader verification | Command selection omits build/vet/race/fuzz/diff gates; textual proof truncation lacks structured metadata | Duplicate `DetectPipeline`/legacy verification paths pending caller confirmation | Add an exact-head machine-readable release evidence manifest and truthfully surface all statuses |
| Repair/recovery | Local checkpoint/undo conflict safety and fail-closed permissions | Durable task resume and GUI steering | Retry taxonomy, route diversity, partial rollback reporting, and process-restart undo are incomplete | Prompt recovery hints would duplicate a future durable recovery ledger | Test and then persist side-effect/recovery metadata; do not replace checkpoint safety prematurely |
| Performance | Deterministic local microbenchmarks exist | Bounded output/context controls | No current v3-v4 process envelope, RSS, cold/warm startup, or long-horizon measurements | Repeated large-output scans and unbounded summary construction may waste CPU | Build a cold/warm first-turn envelope benchmark before claiming performance gains |
| Security | Safe/Fast permission gate, workspace containment checks, allowlisted MCP environment | Tool output is partly labeled untrusted | Filesystem writes remain TOCTOU-sensitive; verification/git/installer environments and raw MCP results are not uniformly isolated | Automatic `curl \| bash`/global installer fallbacks are high-risk default surface | Centralize subprocess policy with sanitized environment, hardened git flags, and descriptor-relative writes |
| Concurrency | Goal ABA defense and save-before-publish invariants | Unix/Windows lock primitives and stale callbacks | Cancellation/event ordering, cross-surface races, and parallel tool dependencies lack current stress evidence | New orchestration would add complexity before invariants are measured | Add barrier-driven cancellation/save/publish/reconnect harness and run race tests |
| Beginner UX | Safe default, visible progress, action summaries, and undo affordance | Scoped confirmations and readable CLI error structure | Provider jargon, inconsistent failure paths, and untested rendered interaction | First-run attempts several optional provider installs; advanced cards/side rail compete with first success | Make first run one path: folder → Codex → Safe → first useful result; defer optional providers/features |
| GUI | Stale-turn guards, permission generations, and bounded SSE server | Lifecycle and undo wiring | Client can render inconclusive/skipped verification as success; reconnect/event-loss recovery is weak | Narrow events trigger broad reloads; side chat bypasses core task/evidence path | Make verification status canonical and render four distinct states with focused browser coverage |
| TUI/headless | Headless stdout/stderr and fail-closed permission behavior; resume state | CLI dispatch and exit classes | Non-TTY/TUI behavior and session-save failures are not proven or always surfaced | `stdioHandler` stream/discard path is a deletion candidate pending caller proof | Surface persistence failures and test subprocess exit/EOF/signal contracts |
| Maintainability/deletion | Existing safety primitives are localized enough to preserve | Dependency/build surface is understandable | GUI server is 2,702 lines; GUI/TUI routing is duplicated; docs retain stale benchmark anchors | In-repo-unused `repomap.Generate`, `repomap.Build`, and `verify.DetectPipeline` aliases | Delete only after caller/API confirmation; first reduce duplicated control-plane semantics |

## Cross-cutting scorecard

### Already excellent

- The product still honors the tiny, local-first, single-agent identity. No
  daemon, watcher, embedding index, multi-root service, or user-facing swarm
  was added.
- Goal revisions, tombstones, atomic persistence, save-before-publish, bounded
  task state, pair-safe compaction, Safe/Fast permissions, and local checkpoint
  safety are valuable foundations.
- Verification does not treat missing evidence as a pass in its core model.

### Merely adequate

- Repo mapping, search, scope detection, task inference, repair retries,
  GUI/TUI/headless sharing of the agent loop, and deterministic microbenchmarks
  work for the common small-repository path.
- Existing unit and acceptance tests cover many local invariants, but they are
  not evidence of live provider, browser, rendered UI, or multi-hour behavior.

### Brittle

- Outcome authority is split: generic criteria, legacy verification, newer
  evidence, goal text, and `Goal complete:` presentation can drift.
- Current-head/dirty-tree provenance, cross-process writes, route-aware
  recovery, event ordering, and filesystem TOCTOU boundaries are incomplete.
- GUI verification status is especially high impact: the server emits an
  unresolved status for inconclusive/skipped evidence while the client styles
  every non-failure as a pass (`internal/gui/server.go:2002-2017` and
  `internal/gui/web/app.js:2210-2228`).

### Wasteful

- Duplicate surface admission/inference and separate side-chat orchestration.
- Duplicate stale-output reduction paths and duplicate task/evidence models.
- Optional provider installation and advanced UI concepts in the first-run
  path.
- Compatibility aliases are deletion candidates, but external API usage is
  `UNVERIFIED`; do not delete them speculatively.

## Missing proof that blocks a release claim

The following remain `UNRECORDED` or `UNVERIFIED` and must not be implied by
green unit tests or hosted compile/test CI:

- v3-versus-v4 outcome quality for vague, multi-file, debugging, refactoring,
  security, and long-horizon tasks;
- rendered GUI setup, verification, reconnect, permission, undo, and recovery
  journeys; TUI and headless parity under EOF, signals, and save failures;
- restart/resume/steer behavior after process termination or changing goals;
- cancellation/save/publish ordering, goroutine leaks, cross-process writers,
  and Windows runtime persistence;
- provider-token accuracy, RSS/startup/first-turn envelopes, long-session
  growth beyond the bounded session record, and large-output CPU/allocation
  behavior;
- symlink-swap/TOCTOU, child-environment secret leakage, git hook/textconv
  execution, MCP prompt injection, installer safety, and dependency/SBOM
  evidence;
- fresh hosted security scanning. A prior hosted security scan could not start
  because its SQLite database was read-only; no hosted scan is claimed here.

## Deletion queue

These are candidates, not approved deletions:

1. `repomap.Generate`, `repomap.Build`, and `verify.DetectPipeline` after
   external API compatibility is confirmed.
2. The separate GUI side-chat path if a user-value benchmark does not justify
   its task/evidence bypass.
3. Duplicate GUI/TUI admission wrappers after a shared outcome transition path
   has equivalent coverage.
4. Prompt-only repair diversity hints after structural route/recovery evidence
   exists.
5. Optional provider installers and advanced first-run cards from the default
   setup path.

## Completed and selected slices

The focused GUI verification-truthfulness change and workspace-bound evidence
slice are merged. The durable session boundary is also merged: records are
capped, transient prompts and orphaned tool results are removed, newest history
is retained, oversized loads are rejected before parsing, and legacy records
are normalized before resume.

The next proposals should compete on measured evidence: exact-head repository
provenance, a machine-readable release evidence manifest, and deterministic
concurrency/recovery harnesses. None of those proposals is a release claim.
