# Picogent v4 discovery scorecard

Status: refreshed on 2026-09-03 against the exact `main` merge
`c54501f03a44b420b61f04762461e19011c7b93e` after the outcome, recovery, GUI,
undo-publication, lifecycle, proof-continuity, cancellation, hosted
attestation, TUI recovery, setup, restore-inspection, and workflow action-pin
slices through PRs #297, #298, #299, #301, #303, #318, #320, #343, #347,
#349, #351, #353, #355, #357, #359, #363, and #386 merged. The independent
release-evidence reconciliation is tracked in issue #356; prior audit
observations and their unverified boundaries are preserved below.

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
| Architecture | Tiny local-first, single-agent boundary; bounded state; shared outcome/turn contract; the GUI companion path is removed | Compact `taskstate` model | Lifecycle and persistence semantics remain distributed across `agent`, `goal`, `verify`, and the surfaces | Remaining duplicate surface orchestration and repeated admission/inference | Collapse or justify remaining duplicate control paths; do not add a second planner or index |
| Agent reasoning | Evidence-gated completion, stale-goal protection, and durable intent/turn evidence | Bounded repair loop and durable context | Keyword intent inference; repair diversity is prompt advice rather than route enforcement | Repeated admission/inference logic in GUI, TUI, and headless | Enforce route diversity structurally only if recovery evidence justifies the added state |
| Intent/outcome | Monotonic goal revisions, tombstones, atomic clear, and criterion-bound completion proof | Permission boundary and bounded criteria | Template-only definition of done; ambiguity, negation, and conflicting goals are not structured | Duplicate `Steps`/`DefinitionOfDone` and `Verification`/`Evidence` representations | Keep the taskstate criterion/evidence gate authoritative before expanding outcome features |
| Memory | Bounded task and session records with save-before-publish; cooperative cross-process CAS/rebase coverage; structural value-aware session retention; L-lane live-window candidate evidence | Causal learning remains small and advisory | Live structural selection is unmarked by default; session restart/reconnect recovery remains unverified | Transcript-shaped session storage duplicates structured task/evidence state; live ranking costs more allocations than a tail | Measure production-shaped live retention and restart behavior before widening retention inputs |
| Context efficiency | Pair-safe compaction; 8,192-character durable context; failure signals retained; bounded 32 KiB summary input; bounded value-aware live selection | Deterministic stale-output reduction | Token estimates are rough and not provider-calibrated; lexical priority misses structured/non-English failures | Live value-aware ranking is materially slower and allocation-heavy versus the recency control | Keep the structural benefit only if production-shaped measurements justify its overhead |
| Repo intelligence | On-demand deterministic map with exact bounded provenance and no daemon/index/watcher | Bounded search and map output | Nested roots and fallback semantics still need broader behavioral evidence | Duplicate phrase/default command tables | Exercise provenance refresh at admission and after mutation across supported surfaces |
| Verification | Explicit `PASS`/`FAIL`/`INCONCLUSIVE`/`SKIPPED`; criterion-bound evidence; targeted-to-broader stages; exact-head manifest | Changed-file cap forces broader verification; hosted matrix runs vet, bounded fuzz gates, and a validated release-gate ledger | Task-local verification and release-evidence projections remain distinct; textual proof truncation lacks structured metadata | Legacy verification paths pending caller confirmation | Keep the exact-head manifest, taskstate gate, and CI quality ledger aligned without broadening completion authority |
| Repair/recovery | Local checkpoint/undo conflict safety, fail-closed permissions, and durable latest-turn undo | Durable task resume, GUI steering, and crash-after-write recovery evidence | Retry taxonomy, route diversity, partial rollback reporting, and hostile external-writer behavior remain incomplete | Prompt recovery hints would duplicate a future durable recovery ledger | Test and then persist side-effect/recovery metadata; do not replace checkpoint safety prematurely |
| Performance | Deterministic local microbenchmarks, bounded long-session persistence through 256 turns, fresh/warm child-process RSS envelopes, and a 96-turn live-retention stress fixture exist | Bounded output/context controls | Provider-token accuracy, cross-surface startup, and stable cross-platform budgets remain unmeasured | Live value-aware selection adds a large allocation/latency delta over the recency control; repeated large-output scans and broad provenance passes may waste CPU | Profile production-shaped retention overhead and rerun v3-v4 comparisons before claiming performance gains |
| Security | Safe/Fast permission gate, workspace containment checks, allowlisted MCP environment | Hosted `govulncheck` now scans the dependency graph; tool output is partly labeled untrusted | Filesystem writes remain TOCTOU-sensitive; verification/git/installer environments and raw MCP results are not uniformly isolated | Automatic `curl \| bash`/global installer fallbacks are high-risk default surface | Add hostile runtime evidence and preserve explicit limits around dependency reachability and path races |
| Concurrency | Goal ABA defense, save-before-publish invariants, hosted Linux race coverage, bounded GUI active-turn reconnect evidence, barrier-driven cancellation/save/publish stress, cooperative cross-process writers, trace lock-holder death recovery, sustained bounded trace retention, first-use lock creation recovery, project-registry transactions, and cross-surface lifecycle checkpoints | Unix/Windows lock primitives and deterministic recovery harnesses | Hostile process death outside the trace lock handoff, rendered cross-surface event ordering, and filesystem races lack complete stress evidence | New orchestration would add complexity before invariants are measured | Extend cross-process evidence only where it proves a user-visible recovery invariant |
| Beginner UX | Safe default, visible progress, action summaries, and undo affordance | Scoped confirmations and readable CLI error structure | Provider jargon, inconsistent failure paths, and untested rendered interaction | First-run attempts several optional provider installs; advanced cards/side rail compete with first success | Make first run one path: folder → Codex → Safe → first useful result; defer optional providers/features |
| GUI | Stale-turn guards, permission generations, bounded SSE server, four-state verification presentation, hostile wire coverage, bounded owned-browser reconnect/recovery and long-horizon fixture evidence, HTTP-boundary shutdown/save-failure lifecycle evidence, and hosted Windows console-control cancellation at the server boundary | Lifecycle and undo wiring; one local rendered reconnect path plus one bounded exact-head long-horizon browser run | Live-provider behavior, broad permission/undo semantics, full recovery, and cross-platform rendered journeys remain unverified | Narrow events trigger broad reloads; remaining admission wrappers duplicate task/evidence routing | Add owned-browser permission, undo, full-recovery, and cross-platform rendered evidence before release claims |
| TUI/headless | Headless stdout/stderr, fail-closed permission behavior, resume state, explicit cleanup/persistence errors, and local macOS EOF/signal/save-failure evidence | CLI dispatch and exit classes | Non-TTY/rendered TUI behavior and cross-platform EOF/signal behavior remain unproven | `stdioHandler` stream/discard path is a deletion candidate pending caller proof | Add platform-appropriate subprocess/rendered evidence before changing the shared runtime boundary |
| Maintainability/deletion | Existing safety primitives are localized enough to preserve | Dependency/build surface is understandable | GUI server is 2,702 lines; GUI/TUI routing remains duplicated | Repeated surface/control paths and stale benchmark anchors | Remove or simplify remaining duplicate control paths only after caller/API confirmation |

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

- The taskstate criterion/evidence gate is now the completion authority and is
  projected through the Outcome Engine and the three entry points. Generic
  legacy fields, goal text, and `Goal complete:` presentation still exist as
  compatibility inputs and require continued drift testing.
- Current-head/dirty-tree provenance is now captured by the exact-head
  manifest, cooperative cross-process writes and trace lock-holder death
  recovery have deterministic coverage; event ordering, hostile process death
  outside that narrow trace handoff, and filesystem TOCTOU boundaries remain
  incomplete.
- GUI verification events now preserve PASS, FAIL, SKIPPED, and INCONCLUSIVE;
  hostile HTTP/SSE wire coverage and redaction coverage are also merged. PR
  #197 records bounded macOS/arm64 BrowserOS evidence for reconnect,
  active-turn prompt preservation, and durable transcript recovery against a
  deterministic local provider stub; PR #386 records the hosted Windows
  console-control cancellation fixture at the GUI server boundary. Live-provider,
  permission, undo, full recovery, and cross-platform rendered behavior remain
  unverified.

### Wasteful

- Duplicate surface admission/inference and stale benchmark anchors.
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
  and transcript-recovery path are recorded; HTTP-boundary GUI lifecycle
  shutdown/reconnect/save-failure evidence is also recorded, including hosted
  Windows console-control cancellation for the GUI shutdown fixture in PR #386.
  Live-provider behavior, permission, undoable file changes, full recovery, and
  cross-platform rendered behavior remain unverified. Local macOS TUI/headless
  parity under EOF, signals, and save failures is recorded; Windows
  EOF/save-failure behavior and rendered behavior remain unverified;
- full restart/resume/steer behavior after process termination or changing
  goals; bounded fresh-process session attachment now records stale active-turn
  recovery with explicit route, evidence, and stop-reason metadata, and latest-
  turn undo now survives the documented post-write crash boundary;
- hostile cancellation/process-death ordering outside the narrow trace
  lock-holder handoff, goroutine leaks, rendered cross-surface reconnects, and
  Windows runtime persistence. Cooperative cancellation/save/publish and
  cross-process writer scenarios are covered by deterministic tests; the MCP
  lease, TUI/CLI cleanup, GUI shutdown admission, and turn-wait slices are
  unit-, fresh-process-, and hosted-CI-covered, but none establish broader
  hostile process-death or rendered runtime proof;
- provider-token accuracy, cross-platform performance budgets, first-turn
  envelopes, long-session growth beyond the bounded session record, and
  large-output CPU/allocation behavior;
- symlink-swap/TOCTOU, child-environment secret leakage, git hook/textconv
  execution, MCP prompt injection, installer safety, and dependency/SBOM
  evidence beyond the hosted vulnerability reachability scan;
- independent release audit, SBOM/production-binary signing, and hostile/deep
  security scanning. Hosted release attestations are now directly observed for
  the bounded gate and manifest subjects, but the verification manifest remains
  `UNVERIFIED`; hosted `govulncheck` is not a substitute for those runtime and
  supply-chain audits. Current CI workflow action references are pinned to
  verified immutable release commits by PR #363, which narrows action-drift
  risk without establishing any of the release, runtime, or signing claims
  above.

## Deletion queue

These are candidates, not approved deletions:

The former separate GUI side-chat path was removed by PR #242 after caller/API
confirmation; it is no longer a deletion candidate.

1. Duplicate GUI/TUI admission wrappers after a shared outcome transition path
   has equivalent coverage.
2. Prompt-only repair diversity hints after structural route/recovery evidence
   exists.
3. Optional provider installers and advanced first-run cards from the default
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
hostile-runtime, SBOM, or production-release claims. PR #320 now publishes and
verifies a GitHub/Sigstore attestation for the two exact JSON subjects; the
independent audit in PR #322 records the observed provenance and keeps the
remaining release boundaries explicit.

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

PR #235 merged as `0dc4c6d425747bc18474009d52cb05906858124d`. It serializes
project-registry read/modify/write transactions with a secure sidecar lock,
publishes bounded YAML atomically while rejecting symlink targets, and rejects
oversized serialized state before replacing an existing registry. The GUI now
prepares project selection, replaces the runtime, and commits the registry with
a compare-and-swap; failed runtime construction leaves the prior selection
untouched, and removing the active project is rejected. Focused repeated tests,
full tests, full race tests, vet, Windows/Linux package cross-compilation, and
the hosted Ubuntu, macOS, Windows, security, and release-evidence gates passed.
This is project-registry transaction evidence, not a general hostile
filesystem-race or release-readiness claim.

PR #242 merged as `29e4b1cb7623022fc4316704cbaffb43fae8a661`. It removes the
duplicate GUI companion control plane: the `/api/sidechat` handler and state,
side-stream events, companion drawer/FAB, and retired prompt/cache paths. The
primary chat/help/task flows remain in place, and the focused, full, race,
vet, build, Ubuntu, Windows, macOS, security, and release-evidence checks
passed. Executable browser-level primary event coverage remains a follow-up
tracked in issue #243.

## Current-head reconciliation (2026-09-03)

The scorecard is now reconciled with the exact audited `main` head
`993258f4b97d196fd7c44cca78c235080fd062e9`. The following bounded slices are
confirmed by the parent issue's current evidence ledger and the completed
continuity child lanes. The action-pin entry at the end is current-main
evidence; older audit entries retain the limits that applied at their heads:

- PRs #249 and #250 establish the executable cross-surface completion matrix
  and route CLI, GUI, and TUI retirement through the shared
  `agent.CompletionProjection` / `Result.CompletionGate`.
- PRs #252, #254, #256, #258, and #260 cover fresh-process follow-up recovery,
  task-progress proof, intent-change invalidation, truthful provenance, and
  intent-verification routing.
- PRs #262, #264, and #266 record deterministic benchmark/recovery evidence,
  crash-after-write steering recovery, and durable Outcome Engine transition
  guidance.
- PRs #269, #273, #276, and #278 cover transcript redaction, durable latest-
  turn undo, state/event redaction, and hostile HTTP/SSE wire behavior.
- PR #279 records the portability boundary for conditional undo publication
  and keeps the arbitrary same-UID final-path race explicitly unverified.
- PR #283 defines the cross-surface lifecycle scenario vocabulary, PR #284
  adds headless/TUI interruption and persistence-failure evidence, and PR #285
  adds GUI shutdown, reconnect, and persistence-failure evidence. The GUI
  fresh-process result is local macOS HTTP-boundary evidence; rendered-browser
  behavior and Windows child SIGINT remain explicitly unverified.
- PR #297 adds the deterministic taskstate continuity matrix for queued and
  resumed turns, including revision chronology, stale-proof rejection, and
  fresh criterion-bound rebinding.
- PR #298 adds the GUI continuity regression for FIFO queued admission across
  agent rebuild and same-session reload, including durable mutation
  invalidation and fresh proof projection.
- PR #299 reconciles this scorecard and the parent evidence ledger after the
  continuity lanes; PR #301 adds the hosted cancellation-harness startup
  variance guard. The benchmark comparison remains anchored to its named
  historical v4 checkpoint. PR #303 adds standalone attribution controls and a
  production-shaped durable scripted-turn control; current optimization work
  remains tracked in issue #302.
- PR #318 adds the bounded local Ed25519 release-attestation contract with
  fail-closed malformed and absent-evidence semantics. PR #320 adds the
  hosted GitHub/Sigstore publication and exact-subject verification path;
  post-merge run `33581839408` directly exposes both attestations for merge
  SHA `3900ce229440bd1ece64c9d2cf15960aa471bdcd`. PR #322 records the fresh
  independent artifact recheck and preserves the `UNVERIFIED` manifest,
  SBOM, production-release, and runtime boundaries.
- PR #343 adds the TUI process-kill fixture and reconciles bounded crash-
  recovery evidence across the existing headless, GUI, and TUI seams.
- PR #347 exposes the existing recovery controls independently in TUI help and
  its wide footer. Its source head is `043af13516db7983e1e120418f0af746a1b49d41`
  and its merge commit is `a2c305da7be0d132abc410d6499f3643f3afd63c`; the
  post-merge main run `33687818060` passed all hosted gates.
- PR #349 keeps those recovery controls complete on narrow terminals by
  selecting whole width-fitting candidates rather than emitting partial
  commands. Its source head is `f5e924f45e7800c5b8c96c84c6fd97f565107519`,
  its merge commit is `81b5efeddddbe59f9dd832b961ccf770e176a984`, and its
  post-merge main run `33691443732` passed Ubuntu, Windows, macOS, security,
  and release-evidence.
- Issue #344 records the independent release audit at the prior `main` SHA
  `a3c4ccf1a20add17724243521f088e1c4ec11ab2` using post-merge run
  `33683301465`. The hosted gate ledger and both Sigstore subjects verify, but
  the signed manifest is `INCONCLUSIVE` because its bounded broader check was
  killed and targeted coverage was skipped. SBOM, production-release,
  live-provider, rendered-platform, hostile-runtime, and release-authorization
  claims remain outside the evidence; moving action pins were also unverified
  at that historical head.
- PR #355 closes the bounded GUI fresh-process teardown issue #354. Its source
  head is `8a3a347ea111818c164b2eac4782d9c861a84c15`, its merge commit is
  `92e13c590e5dde15e7e2a9849dc2cc7c40f7d3c2`, and post-merge run
  `33697379530` passed Ubuntu, Windows, macOS, security, and
  release-evidence. The test-only helper bounds child and stdout cleanup; it
  does not establish a production cancellation, rendered, live-provider, or
  release-readiness claim.
- Issue #356 records the independent release audit at the prior audited
  `main` SHA `92e13c590e5dde15e7e2a9849dc2cc7c40f7d3c2` using post-merge run
  `33697379530`. The hosted gate ledger and both Sigstore subjects verify; the
  manifest's broader check passed 42 tests, while targeted coverage was
  skipped and coverage remained `UNVERIFIED`. SBOM, production-release,
  live-provider, rendered-platform, hostile-runtime, and release-authorization
  claims remain outside the evidence; moving action pins were also unverified
  at that historical head.
- Issue #358 is closed by PR #359, which reuses restore preflight state to
  avoid a duplicate inspection while retaining the final secure
  compare-before-publish and conflict checks. Its source head is
  `168b4f9d0538cf8d5a921dd8b0ab8a3b3d5dda96`, merge commit is
  `41b95d6f37efc5f25e881a9da10ee0909dfa389a`, and PR/post-merge runs
  `33700420109`/`33700777570` passed the hosted gates. Targeted restore
  allocations decreased from 100 to 77 per operation; wall-clock change is
  inconclusive under filesystem variance. This remains bounded local control
  evidence, not a broad performance or release-readiness claim.
- Issue #362 is closed by PR #363. Its source head is
  `3b649cd9bd9bf1a2e884421a1b8f4c5133870ea2`, its merge commit is
  `27fb38b20aec428ab53912a7e4fec4fd82a2985e`, its PR checks passed in run
  `33704387165`, and post-merge `main` checks passed in run `33704880172`.
  Every hosted action reference in `.github/workflows/ci.yml` now uses a
  40-character immutable commit SHA mapped to a verified release tag:
  `actions/checkout` `v4.4.0` →
  `11d5960a326750d5838078e36cf38b85af677262`, `actions/setup-go` `v5.6.0` →
  `40f1582b2485089dde7abd97c1529aa768e1baff`, `actions/setup-node` `v4.4.0`
  → `49933ea5288caeca8642d1e84afbd3f7d6820020`, and
  `actions/upload-artifact` `v4.6.2` →
  `ea165f8d65b6e75b540449e92b4886f43607fa02`; the existing `actions/attest`
  SHA pin is unchanged. This closes the moving-action-pin gap for the current
  CI workflow only; SBOM, production signing, live-provider, rendered-platform,
  hostile-runtime, and release-authorization claims remain unverified.
- PR #367 delivers the small rendered long-horizon contract at source head
  `7482d3d` and merge commit `d28a807d964088305346689287d8b329ba7916d3`;
  PR run `33708035569` and post-merge `main` run `33708627256` passed all
  hosted gates.
- PR #369 delivers the medium task-owned rendered fixture at source head
  `ca8ad9562ab090aabcef1575f5eb38bb17f5e7d7` and merge/current `main` commit
  `993258f4b97d196fd7c44cca78c235080fd062e9`; PR run `33711166860` and
  post-merge `main` run `33711511650` passed security, Ubuntu, macOS, Windows,
  and release-evidence.
- Issue #370 records the bounded large direct-browser checkpoint against that
  exact clean merge source. The task-owned BrowserOS run directly observed
  permission, mutation, verification, steering invalidation, and reload
  fail-closed behavior through the deterministic local fixture. The record
  keeps screenshot path `UNRECORDED` and live-provider, cross-platform,
  hostile-writer, broader crash-window, and release-authorization boundaries
  explicitly `UNVERIFIED`.

## Retention L-lane candidate (2026-09-03)

The M retention lane from issue #405 is closed by PR #416. Its source commits
`c77c1a7`, `86e4870`, and `1a3f076` merged to `main` as `5a45b46`; hosted run
`33816820403` passed the Ubuntu, Windows, macOS, security, and release-evidence
jobs. This is the prerequisite evidence for the L lane, not a live-provider
quality claim.

The L candidate is implemented on top of that exact main merge by source
checkpoints `9ba07f7` and `af3ca8c` on branch
`codex/v4-retention-live-l-406`, with a documentation-only clarification
checkpoint `88fae5c`. It routes the existing `internal/ctxmgr.Manage` windowing
boundary through the S contract's bounded structural projection. The candidate
preserves the system prompt, newest request, newest complete turn, and complete
tool exchanges while keeping the one-agent interface and all permission,
undo, provider, and surface boundaries unchanged.

Direct local evidence on an Apple M3 arm64 Mac with Go `1.26.6`:

- the focused window benchmark retained the historical target in `1.000` of
  value-aware runs and `0.000` of recency-only runs; value-aware timing was
  `23.3–30.6 µs/op` versus `0.296–0.378 µs/op`, with `65,632` versus `2,176`
  bytes/op and `226` versus `2` allocations/op;
- the 96-turn fixture reduced `384` raw messages to `18` managed messages at
  `660` managed tokens, retained complete historical exchange `read-088`, and
  the recency-only control did not; and
- the full long-horizon stress benchmark reached `2.51–2.91 s/op` and
  `214.7–216.1 MiB/op`, so this candidate records an allocation-heavy stress
  cost rather than claiming an end-to-end performance improvement.

Decision: keep the L integration as a conditional ship candidate because the
retained-context benefit is reproducible and bounded. Do not promote this to a
broad context-quality or release claim: live markers are currently unmarked,
live-provider behavior is unverified, and rendered/cross-platform runtime
behavior beyond hosted CI remains unverified. If production-shaped evidence
does not justify the measured overhead, delete the live integration and retain
the S/M contract.

The #296 medium lane is intentionally not opened: the small contract lane and
the GUI integration lane exposed no production behavior defect. The focused
post-merge race suite passed on the exact head with
`go test -race ./internal/taskstate ./internal/agent ./internal/session ./internal/gui -count=1`.
Any future production seam should be tracked as its own issue-linked medium
lane rather than adding a speculative correction to #296.

These slices prove focused contracts and deterministic local or hosted checks;
they do not establish live-provider quality, rendered cross-platform behavior,
full hostile lifecycle recovery, SBOM or production-release readiness, or a
general filesystem race guarantee. The hosted attestation itself is confirmed
for the bounded evidence subjects, while the independent audit remains
`INCONCLUSIVE` for release authorization. Parent #246 remains open while those
boundaries remain unresolved.
