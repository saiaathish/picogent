# Picogent v4 invention lab

Status: the bounded experiment record for issue [#806](https://github.com/saiaathish/picogent/issues/806) is complete; PR review and hosted checks are still required. This is not a v4 quality verdict or release authorization.

The product source inspected was `b291e2db3c56984e4f7e06b35c9582b5b9e1f391` (the #803 merge). The test-only experiment harness was committed and tested at `6ba2714a321d87ed6bdd75c46d8310554e154d1f`. Environment: Apple M3, Darwin 27.0.0 arm64, Go `1.26.9`. Review inspected the pre-existing untracked `graphify-out/` metadata; it remains excluded from this change and was not staged or removed.

The [v3 invention record](V3-INVENTION-LAB.md) remains historical and unchanged. It records Repair-Diversity as shipped and four rejected candidates. This ledger does not rename or repeat those prototypes. The earlier 80/80 v3/v4 outcome-quality artifact remains `INCONCLUSIVE`; a matched real-task evaluation is separately tracked in [#807](https://github.com/saiaathish/picogent/issues/807).

## Candidate ledger

| # | Candidate and user problem | Expected benefit | Novelty versus current behavior | Feasibility and risk | Disposition |
|---|---|---|---|---|---|
| 1 | **Symmetric v3/v4 telemetry bridge.** The old controlled comparison could not compare every baseline metric. | A defensible comparison instead of treating missing telemetry as zero. | The report contract and isolated v3 adapter already exist in `internal/benchmark`; this narrows the gap to metric-presence and duplicate-event accounting. | Medium. Historical raw runs may not contain recoverable events, and inferred values would fabricate evidence. | **Keep for #807.** The deterministic presence prototype passes, but does not supply missing historical or live-task data. |
| 2 | **Outcome-to-diff scope-drift fence.** A turn may edit paths outside an explicit “README only” constraint. | Detect exact-path drift before presenting the outcome as finished. | `internal/outcome.PredictImpact` classifies the changed surface; it does not enforce an exact per-turn allow-list. `taskstate.TurnRecord` already retains a bounded per-turn path list. | Good for explicit file lists, medium overall. Natural-language constraints, generated files, renames, capped path lists, and pre-existing dirty files make attribution easy to get wrong. | **Defer.** Keep the prototype exact-path-only; no semantic scope parser or runtime gate is justified by this fixture. |
| 3 | **External-effect recovery receipts.** A process can stop after a service action but before its acknowledgement is saved. | Tell the user whether an external effect is confirmed, unknown, or not attempted, and avoid claiming undo. | Turn records retain mutation counts and changed paths, not non-file side effects or their reconciliation state. | Medium. A receipt cannot discover whether an unacknowledged action committed; identifiers can also expose sensitive metadata. | **Reject generic replay/undo receipts.** The adversarial lost-ack case remains unknown and must stop for reconciliation. The simpler alternative is to mark such actions non-undoable and warn that the effect may remain. |
| 4 | **Trust provenance through summaries.** Compaction must not turn hostile tool text into trusted instructions. | Preserve the existing trust boundary as old results are shortened. | `llm.Message` keeps tool role/name/call identity; `ctxmgr.DigestStaleTools` already labels condensed output `untrusted tool output`. Other reducers are not thereby proven. | Medium. New taint metadata may duplicate existing labels and could cause routine text to be over-classified. | **Reject a second generic taint layer for the stale-tool digest path.** The adversarial prototype confirms the current label and tool identity survive. Keep this as a regression check; end-to-end provenance across every reducer remains unverified. |
| 5 | **Context-retention cost/quality governor.** Value-aware history can preserve a useful old exchange but cost more than recency-only truncation. | Keep critical history within a predictable CPU and memory envelope. | `ctxmgr.ValueAwareWindow` already ranks complete exchanges; the new 96-turn comparison measures its retention/cost trade-off against `TruncateTail`. | Medium. A cutoff can discard the only relevant old fact; one synthetic fixture is not a production budget. | **Defer.** The old target survives, but the measured allocation delta is large. Require production-shaped repeated profiles and a pre-registered threshold before adding a second runtime policy. |
| 6 | **Constraint/criterion conflict preflight.** A “do not change backend” constraint could conflict with a required backend criterion. | Resolve direct contradictions before editing. | The current contradiction detector compares evidence; it does not decide whether free-text intent constraints contradict criteria. | Medium. Exact structured rules are feasible; semantic matching risks needless questions. | **Hold** for a structured contract and a narrow exact-match fixture. |
| 7 | **Environment-bound verification identity.** The same files can be checked under a different toolchain or operating system. | Make it clearer when old verification no longer describes the current environment. | Task evidence records verification state; the benchmark records host/tool metadata, but a durable environment fingerprint is not established as a task-level invalidation rule. | Medium. Toolchain fingerprints can be noisy and may accidentally capture secrets or machine-specific paths. | **Hold** until a concrete environment change invalidates a real proof. |
| 8 | **Dependency-aware test expansion.** A path-targeted test plan may miss a downstream consumer. | Select a useful targeted check before the existing broader verification stage. | Current verification planning uses changed paths and bounded project signals, not a complete dependency closure. | Stack-specific and incomplete for generated/dynamic dependencies; broad tests remain a safer fallback. | **Reject for now.** No measured missed regression or suite-cost result supports a second dependency graph. |
| 9 | **Partial-restore stop projection.** A conflicted restore must not look like a complete undo on any surface. | Make incomplete recovery visible and prevent continuation from a mixed state. | Restore results already distinguish outcomes; the remaining question is cross-surface projection consistency. | Moderate. A new status could duplicate existing fail-closed behavior. | **Hold** pending a reproducible surface-specific mismatch. |
| 10 | **Content-addressed health-scan reuse.** Repeated inventory scans may waste CPU on unchanged files. | Avoid rescanning an unchanged snapshot. | Project health currently inspects a bounded live snapshot without a persistent index. | Medium. Cache invalidation mistakes would present stale health as current. | **Reject absent a profile** showing health scans are material; a fresh bounded scan is simpler and authoritative. |
| 11 | **Cross-surface lifecycle differential replay.** GUI, TUI, and headless recovery can diverge across the same sequence. | Expose parity defects with one reproducible scenario. | Existing lifecycle tests cover individual transitions; a shared differential replay of the same trace is a separate harness. | Medium. Browser/rendered paths can be flaky and platform-specific. | **Hold** for a concrete parity gap and a deterministic trace. |
| 12 | **First-result friction measurement.** Setup success does not prove a beginner reaches a useful result easily. | Measure the first-run journey before redesigning it. | Setup flow is intentionally short; there is no comparable first-use friction score in the current evidence. | A local stub can count steps but cannot prove live authentication or user comprehension. | **Hold** for fresh rendered/user observation; do not infer success from setup tests. |
| 13 | **Goal-replacement artifact relevance map.** A new goal invalidates old proof even when some completed edits remain useful. | Avoid repeating useful work without carrying stale completion evidence forward. | Outcome replacement invalidates old proof; it does not label old edits “possibly reusable but unverified.” | Medium. Incorrect reuse can preserve changes that now conflict with the user’s goal. | **Hold** until a controlled task demonstrates repeated wasted work. |
| 14 | **Transient-error retry taxonomy.** Provider/tool transport errors and verification failures should not trigger the same retry. | Avoid wasting attempts or duplicating an action after an ambiguous response. | Failure intelligence currently focuses on verification failures; transport-side idempotency remains distinct. | Medium-high. A timeout may occur after an action committed, so a generic retry can duplicate it. | **Hold** for injected transport evidence and explicit idempotency classes. |
| 15 | **Counterexample probes for acceptance criteria.** A passing check may be too weak to distinguish success from a near-miss. | Find shallow acceptance tests before trusting them. | Current evidence binds checks to criteria; it does not generate adversarial counterexamples for each criterion. | Medium. A synthetic counterexample can misrepresent the user’s intent and produce false confidence. | **Hold** until one criterion family has a trusted oracle and bounded counterexample generator. |

## Five deterministic prototypes

The harness is isolated under [`docs/experiments/v4-invention-lab/`](experiments/v4-invention-lab/). It imports existing context-window behavior only for prototypes 4 and 5; none of the harness code is imported by Picogent runtime code. These tests establish narrow deterministic properties, not model behavior.

### 1. Metric-presence parity

The adapter requires one observation for each declared metric on both sides. A measured zero counts as present; a missing or duplicate event makes the pair incomparable. The adversarial case removes `baseline.repair_count` and duplicates `candidate.tokens`.

Observed test line:

```text
metrics-parity: complete=true zero_is_measured=true incomplete=false findings=[duplicate candidate.tokens missing baseline.repair_count]
```

Decision: retain this parity check as scaffolding for #807. It neither reconstructs old telemetry nor establishes a quality win.

### 2. Exact-path scope fence

Given a structured allow-list of `README.md`, the fixture checks turn-attributed changes, a rename represented by both paths, path traversal, a pre-existing dirty file omitted from the turn’s path list, and a capped path list. It flags the exact unauthorized paths and returns `INCONCLUSIVE` when attribution is incomplete.

Observed test line:

```text
scope-fence: baseline_dirty=ignored allowed=IN_SCOPE drift=SCOPE_DRIFT rename=SCOPE_DRIFT traversal=SCOPE_DRIFT capped=INCONCLUSIVE
```

Decision: defer a product change. This prototype does not translate natural-language constraints into a safe allow-list.

### 3. Lost-ack external effect

The state-machine fixture models an action that was attempted, may have committed, and lost its acknowledgement before process recovery. It must become `UNKNOWN_AFTER_CRASH`; the runner may not retry it or claim complete undo. A confirmed action and a not-started action remain distinct.

Observed test line:

```text
effect-receipt: lost_ack=UNKNOWN_AFTER_CRASH retry=false undo_complete=false acknowledged=CONFIRMED not_started=NOT_STARTED
```

Adversarial result and rejection: a receipt records uncertainty but cannot reconcile the external system. Automatically retrying would risk duplicate effects. A generic receipt/replay feature is rejected; the simpler safe behavior is to mark the effect non-undoable, stop, and tell the user what is unknown.

### 4. Stale-tool trust label

The test sends old tool output containing a fake closing marker and hostile instruction through the existing `ctxmgr.DigestStaleTools` path. It checks that the message remains a tool message with its original tool name and call ID, and that the condensed content retains the untrusted label.

Observed test line:

```text
tool-trust-digest: role=tool tool=read_file call_id_preserved=true untrusted_label=true hostile_text_retained=true
```

Adversarial result and rejection: the label and source identity survive this one reducer, so a second generic taint system for this path would duplicate existing behavior. The hostile text remains present and untrusted; this does not prove that every context reducer or live model will handle it safely.

### 5. Ninety-six-turn retention/cost comparison

The fixture places one historical complete tool exchange before 95 later user turns, then compares a 10-message `ValueAwareWindow` with `TruncateTail`. Both preserve the newest turn and its complete tool exchange. The value-aware window retains the historical target; the recency control does not. The retained value-aware output has no orphan tool result.

Observed test line:

```text
retention-96: turns=96 value_aware_target=true value_aware_latest_pair=true value_aware_messages=10 recency_target=false recency_latest_pair=true recency_messages=10
```

One local benchmark sample (100 ms target, one run) measured:

| Strategy | Time/op | Bytes/op | Allocs/op | Target coverage/op |
|---|---:|---:|---:|---:|
| Value-aware | 294,760 ns | 226,384 | 608 | 1.000 |
| Recency-only | 729.2 ns | 2,192 | 3 | 0.000 |

The allocation and timing figures are a machine-specific synthetic signal, not an SLA or product-wide performance claim. This is a clear cost/retention trade-off; it does not justify another policy layer without repeat production-shaped profiling.

A repeat of the same 100 ms benchmark command at the same test-source head measured 250,876 ns/op for value-aware retention and 574.6 ns/op for recency-only; bytes/op, allocs/op, and target coverage matched the table. The variation between two short single-run timings is why no stable latency claim is made.

## Reproduction

The prototype source head was `6ba2714a321d87ed6bdd75c46d8310554e154d1f`. From a clean checkout at that head or a later descendant that preserves the harness:

```sh
env GOMAXPROCS=1 GOMEMLIMIT=1GiB GOFLAGS=-p=1 go test ./docs/experiments/v4-invention-lab -run '^TestPrototype' -count=1 -v
env GOMAXPROCS=1 GOMEMLIMIT=1GiB GOFLAGS=-p=1 go test ./docs/experiments/v4-invention-lab -run '^$' -bench '^BenchmarkPrototypeRetention96Turn$' -benchtime=100ms -benchmem -count=1
```

Both commands passed at the recorded source head. The first reported five passing deterministic checks. The second produced the sample table above; a repeat produced the timing sample described above. They use no provider, network, browser, or external repository. No paid API calls or live-provider claims were used for issue #806.

## Evidence boundary and next work

- The 15-item ledger and five fixtures do not show that v4 produces better code than v3.
- The 80/80 controlled-transcript artifact remains `INCONCLUSIVE`; it is not a broad quality result.
- The matched Go/frontend/Python real-task comparison, repeated runs, objective acceptance checks, and blind review remain in #807. Use the existing Codex account/session only, as scoped there; do not substitute mocks for a lane that cannot run.
- Test output is local deterministic evidence. Hosted CI and independent PR review are still required before this issue can close.
- The overall v4 goal remains active. No production behavior changed in this documentation/experiment slice.
