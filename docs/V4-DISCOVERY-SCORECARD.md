# Picogent v4 discovery scorecard

Status: refreshed on 2026-08-31 against exact `main` head
`6935b37e4b9bb6b1178bc8f9ead8d89b71e6bc13` after PRs #219–#233 merged.

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
| Architecture | Tiny local-first, single-agent boundary; bounded state; shared outcome/turn contract | Compact `taskstate` model | Lifecycle and persistence semantics remain distributed across `agent`, `goal`, `verify`, and the surfaces | Separate GUI side-chat path and repeated surface orchestration | Collapse or justify the separate side-chat control plane; do not add a second planner or index |
| Agent reasoning | Evidence-gated completion, stale-goal protection, and durable intent/turn evidence | Bounded repair loop and durable context | Keyword intent inference; repair diversity is prompt advice rather than route enforcement | Repeated admission/inference logic in GUI, TUI, and headless | Enforce route diversity structurally only if recovery evidence justifies the added state |
| Intent/outcome | Monotonic goal revisions, tombstones, atomic clear | Permission boundary and bounded criteria | Template-only definition of done; ambiguity, negation, and conflicting goals are not structured | Duplicate `Steps`/`DefinitionOfDone` and `Verification`/`Evidence` representations | Make criteria and criterion evidence authoritative before expanding outcome features |
| Memory | Bounded task and session records with save-before-publish; cooperative cross-process CAS/rebase coverage | Causal learning remains small and advisory | FIFO retention is not value-aware; session restart/reconnect recovery remains unverified | Transcript-shaped session storage duplicates structured task/evidence state | Make retention value-aware after measuring restart and compaction behavior |
| Context efficiency | Pair-safe compaction; 8,192-character durable context; failure signals retained; bounded 32 KiB summary input | Deterministic stale-output reduction | Token estimates are rough and not provider-calibrated; lexical priority misses structured/non-English failures | Repeated stale skeletonization/deduplication passes | Measure token estimates and retention value before adding another compaction mechanism |
| Repo intelligence | On-demand deterministic map with exact bounded provenance and no daemon/index/watcher | Bounded search and map output | Nested roots and fallback semantics still need broader behavioral evidence | Duplicate phrase/default command tables | Exercise provenance refresh at admission and after mutation across supported surfaces |
| Verification | Explicit `PASS`/`FAIL`/`INCONCLUSIVE`/`SKIPPED`; targeted-to-broader stages; exact-head manifest | Changed-file cap forces broader verification; hosted matrix now runs vet and bounded fuzz gates | Task-local verification does not yet aggregate every release gate; textual proof truncation lacks structured metadata | Legacy verification paths pending caller confirmation | Keep the exact-head manifest and CI quality gates aligned; add structured gate results without broadening completion authority |
| Repair/recovery | Local checkpoint/undo conflict safety and fail-closed permissions | Durable task resume and GUI steering | Retry taxonomy, route diversity, partial rollback reporting, and process-restart undo are incomplete | Prompt recovery hints would duplicate a future durable recovery ledger | Test and then persist side-effect/recovery metadata; do not replace checkpoint safety prematurely |
| Performance | Deterministic local microbenchmarks, bounded long-session persistence through 256 turns, and fresh/warm child-process RSS envelopes exist | Bounded output/context controls | Provider-token accuracy, cross-surface startup, and stable cross-platform budgets remain unmeasured | Repeated large-output scans and broad provenance passes may waste CPU | Profile retention/provenance overhead and rerun v3-v4 comparisons before claiming performance gains |
| Security | Safe/Fast permission gate, workspace containment checks, allowlisted MCP environment | Hosted `govulncheck` now scans the dependency graph; tool output is partly labeled untrusted | Filesystem writes remain TOCTOU-sensitive; verification/git/installer environments and raw MCP results are not uniformly isolated | Automatic `curl \| bash`/global installer fallbacks are high-risk default surface | Add hostile runtime evidence and preserve explicit limits around dependency reachability and path races |
| Concurrency | Goal ABA defense, save-before-publish invariants, hosted Linux race coverage, bounded GUI active-turn reconnect evidence, barrier-driven cancellation/save/publish stress, cooperative cross-process writers, trace lock-holder death recovery, sustained bounded trace retention, and first-use lock creation recovery | Unix/Windows lock primitives and deterministic recovery harnesses | Cross-surface event/reconnect behavior, hostile process death outside the trace lock handoff, and filesystem races lack complete stress evidence | New orchestration would add complexity before invariants are measured | Extend cross-process evidence only where it proves a user-visible recovery invariant |
| Beginner UX | Safe default, visible progress, action summaries, and undo affordance | Scoped confirmations and readable CLI error structure | Provider jargon, inconsistent failure paths, and untested rendered interaction | First-run attempts several optional provider installs; advanced cards/side rail compete with first success | Make first run one path: folder → Codex → Safe → first useful result; defer optional providers/features |
| GUI | Stale-turn guards, permission generations, bounded SSE server, four-state verification presentation, and bounded owned-browser reconnect/recovery evidence | Lifecycle and undo wiring; one local rendered reconnect path | Live-provider behavior, permission decisions, undoable changes, full recovery, and cross-platform rendered journeys remain unverified | Narrow events trigger broad reloads; side chat bypasses core task/evidence path | Add owned-browser permission, undo, full-recovery, and cross-platform evidence before release claims |
| TUI/headless | Headless stdout/stderr, fail-closed permission behavior, resume state, and explicit cleanup/persistence errors | CLI dispatch and exit classes | Non-TTY/TUI behavior and cross-platform EOF/signal behavior remain unproven | `stdioHandler` stream/discard path is a deletion candidate pending caller proof | Add subprocess/EOF/signal evidence before changing the shared runtime boundary |
| Maintainability/deletion | Existing safety primitives are localized enough to preserve | Dependency/build surface is understandable | GUI server is 2,702 lines; GUI/TUI routing is duplicated; side-chat bypasses the main task/evidence path | Duplicate control-plane paths and stale benchmark anchors | Remove or simplify duplicate control planes only after caller/API confirmation |

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
  manifest, cooperative cross-process writes and trace lock-holder death
  recovery have deterministic coverage; event ordering, hostile process death
  outside that narrow trace handoff, and filesystem TOCTOU boundaries remain
  incomplete.
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
- hostile cancellation/process-death ordering outside the narrow trace
  lock-holder handoff, goroutine leaks, cross-surface reconnects, and Windows
  runtime persistence. Cooperative cancellation/save/publish and cross-process
  writer scenarios are covered by deterministic tests; the MCP lease, TUI/CLI
  cleanup, GUI shutdown admission, and turn-wait slices are unit- and hosted-
  CI-covered, but none establish broader hostile process-death or rendered
  runtime proof;
- provider-token accuracy, cross-platform performance budgets, first-turn
  envelopes, long-session growth beyond the bounded session record, and
  large-output CPU/allocation behavior;
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
remaining proof work is still open: hostile process death, cross-surface
event/reconnect behavior, v3-versus-v4 outcome/retry measurements, and
owned-browser permission/undo/full-recovery evidence. Cooperative
cross-process writer coverage was added by PRs #222 and #224.

PR #219 merged as `afd787289c61ae65a2adcca6f5eecdfbdc15c2f7`. It preserves a
recoverable active task when turn-close persistence fails and joins that error
with an earlier provider or permission failure, so the primary failure and the
durability failure are both visible. Hosted macOS, Ubuntu, Windows, security,
and release-evidence checks passed.

PR #220 merged as `5f7d31237a4ffd586c27882588a49cdaed78d9f9`. Its
barrier-driven cancellation harness ran 24 isolated probes, checking that
save-before-publish ordering, interrupted-turn metadata, persisted side
effects, and live task state remain consistent after cancellation. The focused
and race test runs, full Go suite, and all hosted macOS, Ubuntu, Windows,
security, and release-evidence checks passed. This is deterministic agent
boundary evidence, not cross-process writer or rendered cross-surface proof.

PR #223 merged as `15ece28a59d244ca5786acae5850b3ec54f61ea6`. It bounds the
provider-bound aggregate summarization input at 32 KiB while retaining both
ends of the older conversation and adds regression coverage for the cap. The
focused, race, and full Go test runs plus all hosted macOS, Ubuntu, Windows,
security, and release-evidence checks passed.

PR #224 merged as `e02caf63da63796990c057b4bc0c293dcce52153`. It hardens the
cross-process agent test harness so child startup failures are observed with
their diagnostics, cleanup does not race a second `Wait`, and readiness/exit
waits remain bounded. The focused repeated test, race test, full Go suite, and
all hosted macOS, Ubuntu, Windows, security, and release-evidence checks passed.

PR #226 merged as `597c7cb1a267686286b17cc180fd5958c6954a40`. It adds a real
trace crash-handoff test: a child acquires the trace lock, is killed, and a
fresh process appends successfully with contiguous sequence numbers. The
focused test ran through 100 crash-handoff repetitions, the combined
trace-process tests through 20 repetitions, and the race variants through 10
repetitions; the full Go suite and all hosted macOS, Ubuntu, Windows,
security, and release-evidence checks passed. This is deterministic proof of
the narrow trace lock-holder handoff, not a claim about session restart,
cross-surface reconnects, hostile filesystem writers, or rendered runtime
behavior.

PR #228 merged as `1ddf40e382b575b9b8bb538f1ddcf619f7b5948a`. It adds a
four-process sustained trace workload that forces JSONL compaction while fresh
processes contend on the lock, then checks the retained tail for contiguous
sequence numbers and the file for the existing size bound. Three normal
repetitions, two race repetitions, the uncached full Go suite, and `go vet`
passed locally; the hosted macOS, Ubuntu, Windows, security, and
release-evidence gates also passed. This is bounded retention and sequence
evidence, not a performance budget or hostile filesystem-writer claim.

PR #230 merged as `41102be6219bcc42978d9f7d06bec962944bd9d7`. It retries a
bounded transient `ENOENT` from first-use descriptor-anchored lock creation
against the same validated parent, preserving the existing symlink boundary.
The previously flaky four-process agent lock test passed 200 repetitions and
the securefile suite passed 10 repetitions; the full Go suite, `go vet`, and
all hosted macOS, Ubuntu, Windows, security, and release-evidence checks also
passed. This addresses first-use lock initialization, not hostile directory
tampering or a general filesystem TOCTOU guarantee.

PR #233 merged as `6935b37e4b9bb6b1178bc8f9ead8d89b71e6bc13`. It adds a fresh
child-process long-session fixture at 1, 64, and 256 turns, checking bounded
trace, learning, evolution, session, and task records plus retained-turn
limits. The 256-turn checkpoint stayed at 90,408 bytes of trace, 53,785 bytes
of learning state, 50,681 bytes of session state, and 5,812 bytes of task
state; the deterministic CLI process envelope measured 607.5 ms fresh-state
and 27.3 ms warm-state with approximately 17.5 MB and 17.3 MB child RSS on
Darwin/arm64. This is local fixture evidence, not a live-provider, GUI,
cross-platform budget, or v3-versus-v4 performance claim.
