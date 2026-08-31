# Picogent v4 discovery scorecard

Status: refreshed on 2026-08-31 against exact `main` head
`83f4d3105bf10c94619eb9544a1b4feb7752040d` after PRs #211–#216 merged.

The required 15-specialty Wave A audit was run in bounded read-only batches on
2026-08-25. The findings below are carried forward and reconciled with the
landed slices since that audit; they are not a new release-readiness claim.
The post-audit lifecycle slices were reviewed against the exact current head
without modifying the protected checkout or its user-owned changes.

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
| Repo intelligence | On-demand deterministic map with no daemon/index/watcher | Bounded search and map output | Short-head/dirty status omits path provenance; nested roots and fallback semantics drift | Duplicate phrase/default command tables | Add a bounded provenance-bearing repo snapshot refreshed at admission and after mutation |
| Verification | Explicit `PASS`/`FAIL`/`INCONCLUSIVE`/`SKIPPED`; targeted-to-broader stages; exact-head manifest | Changed-file cap forces broader verification; hosted matrix now runs vet and bounded fuzz gates | Task-local verification does not yet aggregate every release gate; textual proof truncation lacks structured metadata | Legacy verification paths pending caller confirmation | Keep the exact-head manifest and CI quality gates aligned; add structured gate results without broadening completion authority |
| Repair/recovery | Local checkpoint/undo conflict safety and fail-closed permissions | Durable task resume and GUI steering | Retry taxonomy, route diversity, partial rollback reporting, and process-restart undo are incomplete | Prompt recovery hints would duplicate a future durable recovery ledger | Test and then persist side-effect/recovery metadata; do not replace checkpoint safety prematurely |
| Performance | Deterministic local microbenchmarks and a long-horizon resume envelope exist | Bounded output/context controls | Current refresh shows large repo-map/session allocation overhead; GUI/TUI/headless process envelopes remain unmeasured | Repeated large-output scans and unbounded summary construction may waste CPU | Profile retention/provenance overhead and rerun v3-v4 comparisons before claiming performance gains |
| Security | Safe/Fast permission gate, workspace containment checks, allowlisted MCP environment | Hosted `govulncheck` now scans the dependency graph; tool output is partly labeled untrusted | Filesystem writes remain TOCTOU-sensitive; verification/git/installer environments and raw MCP results are not uniformly isolated | Automatic `curl \| bash`/global installer fallbacks are high-risk default surface | Add hostile runtime evidence and preserve explicit limits around dependency reachability and path races |
| Concurrency | Goal ABA defense, save-before-publish invariants, hosted Linux race coverage, and bounded GUI active-turn reconnect evidence | Unix/Windows lock primitives and deterministic recovery harnesses | Cancellation/event ordering, cross-surface reconnect behavior, and cross-process writers lack complete stress evidence | New orchestration would add complexity before invariants are measured | Add barrier-driven cancellation/save/publish/reconnect harness and run it on every release candidate |
| Beginner UX | Safe default, visible progress, action summaries, and undo affordance | Scoped confirmations and readable CLI error structure | Provider jargon, inconsistent failure paths, and untested rendered interaction | First-run attempts several optional provider installs; advanced cards/side rail compete with first success | Make first run one path: folder → Codex → Safe → first useful result; defer optional providers/features |
| GUI | Stale-turn guards, permission generations, bounded SSE server, four-state verification presentation, and bounded owned-browser reconnect/recovery evidence | Lifecycle and undo wiring; one local rendered reconnect path | Live-provider behavior, permission decisions, undoable changes, full recovery, and cross-platform rendered journeys remain unverified | Narrow events trigger broad reloads; side chat bypasses core task/evidence path | Add owned-browser permission, undo, full-recovery, and cross-platform evidence before release claims |
| TUI/headless | Headless stdout/stderr and fail-closed permission behavior; resume state | CLI dispatch and exit classes | Non-TTY/TUI behavior and session-save failures are not proven or always surfaced | `stdioHandler` stream/discard path is a deletion candidate pending caller proof | Surface persistence failures and test subprocess exit/EOF/signal contracts |
| Maintainability/deletion | Existing safety primitives are localized enough to preserve | Dependency/build surface is understandable | GUI server is 2,702 lines; GUI/TUI routing is duplicated; docs retain stale benchmark anchors | Duplicate control-plane paths and stale benchmark anchors | Delete only after caller/API confirmation; first reduce duplicated control-plane semantics |

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
- Current-head/dirty-tree provenance is now captured by the exact-head
  manifest, but cross-process writes, route-aware recovery, event ordering,
  and filesystem TOCTOU boundaries remain incomplete.
- GUI verification events now preserve PASS, FAIL, SKIPPED, and INCONCLUSIVE.
  PR #197 records bounded macOS/arm64 BrowserOS evidence for reconnect,
  active-turn prompt preservation, and durable transcript recovery against a
  deterministic local provider stub; live-provider, permission, undo, full
  recovery, and cross-platform rendered behavior remain unverified.

### Wasteful

- Duplicate surface admission/inference and separate side-chat orchestration.
- Duplicate stale-output reduction paths and duplicate task/evidence models.
- Optional provider installation and advanced UI concepts in the first-run
  path.
- Compatibility aliases are deletion candidates, but external API usage is
  `UNVERIFIED`; do not delete them speculatively.

## Missing proof that blocks a release claim

The following remain `UNRECORDED` or `UNVERIFIED` and must not be implied by
green unit tests or bounded hosted quality gates:

- v3-versus-v4 outcome quality for vague, multi-file, debugging, refactoring,
  security, and long-horizon tasks;
- rendered GUI setup, verification, and the bounded macOS/local-stub reconnect
  and transcript-recovery path are recorded; live-provider behavior,
  permission, undoable file changes, full recovery, and cross-platform rendered
  behavior remain unverified. TUI and headless parity under EOF, signals, and
  save failures also remain unverified;
- full restart/resume/steer behavior after process termination or changing
  goals; bounded fresh-process session attachment now records stale active-turn
  recovery with explicit route, evidence, and stop-reason metadata;
- cancellation/save/publish ordering, goroutine leaks, cross-process writers,
  and Windows runtime persistence. The MCP lease, TUI/CLI cleanup, GUI
  shutdown admission, and turn-wait slices are unit- and hosted-CI-covered;
  they do not establish hostile process-death, cross-process, or rendered
  runtime proof;
- provider-token accuracy, RSS/startup/first-turn envelopes, long-session
  growth beyond the bounded session record, and large-output CPU/allocation
  behavior;
- symlink-swap/TOCTOU, child-environment secret leakage, git hook/textconv
  execution, MCP prompt injection, installer safety, and dependency/SBOM
  evidence beyond the hosted vulnerability reachability scan;
- hostile/deep security scanning and signed release attestation. Hosted
  `govulncheck` now passes on the dependency graph, but it is not a substitute
  for those runtime and supply-chain audits.

## Deletion queue

These are candidates, not approved deletions:

1. The separate GUI side-chat path if a user-value benchmark does not justify
   its task/evidence bypass.
2. Duplicate GUI/TUI admission wrappers after a shared outcome transition path
   has equivalent coverage.
3. Prompt-only repair diversity hints after structural route/recovery evidence
   exists.
4. Optional provider installers and advanced first-run cards from the default
   setup path.

## Completed and selected slices

The focused GUI verification-truthfulness change, workspace-bound evidence
slice, durable session boundary, restart-recovery slice, and GUI reconnect
evidence slice are merged. The current main also runs cross-platform vet,
bounded security-boundary fuzzing, and a pinned hosted vulnerability scan;
`golang.org/x/sys` is at the fixed `v0.44.0` release.

The hosted `release-evidence` job now validates a bounded `test`/`security`
gate ledger against the exact candidate SHA before uploading evidence. This
closes the missing-job/failed-job artifact path; it does not turn the advisory
verification manifest into a release approval or cover live-provider,
hostile-runtime, or signed-attestation claims.

The restart-recovery slice adds an explicit `process_restart` stop reason and
uses the project-locked `Agent.SetTaskSession` boundary to close a stale active
turn as `recover` with `UNVERIFIED` evidence. The fresh-process long-horizon
harness and taskstate unit contract cover this bounded transition; hostile
process death, live-provider behavior, and cross-platform rendered recovery
remain outside the claim.

PR #197 is merged as `f0f254696f986c172ea192343b133a4e424a3b58`. It adds GUI
history reconciliation after SSE reconnect, protects active-turn local
transcript and activity state while durable history lags, orders asynchronous
session/project refreshes, and records bounded rendered evidence in
`docs/V4-GUI-RECOVERY-EVIDENCE.md`. That evidence is local macOS/arm64 with a
deterministic provider stub; it is not live-provider, permission, undo, full
recovery, hostile-process, or cross-platform rendered proof.

PRs #211–#216 then harden MCP runtime replacement and registry leases, agent
and TUI cleanup, context-driven GUI HTTP shutdown, one-shot CLI cleanup, and
GUI shutdown admission plus admitted-turn waiting. Hosted macOS, Ubuntu,
Windows, security, and release-evidence checks passed for each slice. The
remaining proof work is still open: deterministic cancellation/save/publish
stress, cross-process writers, v3-versus-v4 outcome/retry measurements, and
owned-browser permission/undo/full-recovery evidence.
