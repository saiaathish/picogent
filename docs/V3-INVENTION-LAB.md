# Picogent v3 invention lab

Date: 2026-08-25

Starting point: merged `main` at `aa479e6` (PR #57), after the headless v3
follow-up and the Phase 27 complexity-deletion audit.

This is a bounded experiment record, not a claim about live provider quality.
The prototypes are deterministic and provider-independent. Each candidate was
given a cheap behavioral test, an adversarial critique, and a comparison with
the smallest mechanism already present in Picogent.

## Decision matrix

| Candidate | Prototype result | Adversarial result | Simpler alternative | Decision |
| --- | --- | --- | --- | --- |
| Repair-Diversity Gate | Exact consecutive failure fingerprints were detected; a repeated failure produced a route-change instruction. | Near-identical but non-equal errors are missed; it must remain advisory and bounded. | A fixed retry or the existing failure budget cannot prevent repeating the same repair. | Ship the narrow gate. |
| Evidence Half-Life | Fresh evidence scored `1.000`; evidence 180 days old scored `0.135`. | A stable checkout can become “unverified” only because time passed; clock behavior makes local runs less deterministic. | `ChangeSeq`/`VerifiedChangeSeq` already invalidates proof when the workspace changes. | Delete. |
| Ambiguity Budget | Four ambiguity signals were capped at one clarification. | Automatic questions interrupt routine work and can lock a scope before the user has finished explaining it. | Explicit `--clarify`, scope choices, and per-turn task boundaries. | Delete. |
| Repo Behavior Shadow | A deterministic extension-to-command lookup passed for Go and Rust fixtures. | File extensions do not establish the right command; stale learned commands could run unsafe or irrelevant work. | The bounded repo map and project-learning hints already guide discovery without a second map. | Delete. |
| Verification Route Auction | Historical pass-rate scoring distinguished `3/0` from `1/1`. | A previously passing route can become stale or unsafe; selecting commands from memory weakens live-workspace authority. | `verify.DetectPlan` derives the current route, while causal memory remains advisory. | Delete. |

## Shipped candidate: Repair-Diversity Gate

The durable repair loop already stops after its bounded verification-failure
budget. The new gate handles the narrower failure mode before that limit: if the
same normalized failing evidence appears twice consecutively, the next internal
repair instruction explicitly says to reread the target, change the hypothesis,
and choose a materially different safe route. It never executes a command,
changes permission policy, or adds durable state.

Implementation:

- `internal/agent/durable_task.go` normalizes consecutive failed summaries and
  adds the route-change instruction only on an exact repeated fingerprint.
- `internal/agent/agent.go` wires the advisory gate into the existing repair
  prompt.
- `internal/agent/durable_task_internal_test.go` covers normalization and the
  prompt contract.
- `internal/agent/durable_task_test.go` proves the instruction appears in the
  integrated three-failure loop before the task becomes blocked.

Known limit: the first version intentionally detects only exact normalized
repetition. That keeps the mechanism predictable and cheap; fuzzy clustering
would add more false positives and persistence without current evidence that it
improves repairs.

## Deleted candidates

### Evidence Half-Life

The prototype used `exp(-age/90 days)` as a confidence score. It is attractive
for stale external facts, but task completion proof is local workspace evidence:
the existing change sequence already marks it stale after a mutation. Adding a
wall-clock rule would make an unchanged checkout fail closed for a reason the
user cannot see or reproduce. The causal-memory store already curates old
advisory records, so a second age policy would duplicate semantics.

### Ambiguity Budget

The prototype allowed at most one automatic clarification when several intent
signals were present. Picogent already has explicit `--clarify`, recommended
scope choices, and a task-mode boundary that applies for one turn. Automatically
interrupting ordinary prompts would violate the product identity of inferring
routine intent. The candidate was deleted rather than added as hidden state.

### Repo Behavior Shadow

The prototype mapped extensions to remembered commands. That is too weak a
causal signal: a Go file may belong to a workspace whose correct check is a
package-specific command, and a remembered command may no longer exist.
`repomap` and learned project hints already provide bounded discovery while
keeping the live workspace authoritative. A persistent command map would add
state and invalidation work for little measured benefit.

### Verification Route Auction

The prototype ranked commands by historical pass rate. That ranking is not
permission to execute a command, and promoting it into execution would make
stale memory outrank the current repository. Picogent's live verifier derives a
current targeted/broader plan; causal memory records successful routes only as
advisory context. The safer mechanism won.

## Reproduction boundary

The five prototype assertions were run as an inline JavaScript sandbox on
2026-08-25. They produced:

```text
repair-diversity-gate: PASS repeated=true different=false; ship candidate
evidence-half-life: PASS fresh=1.000 old180d=0.135; delete candidate
ambiguity-budget: PASS signals=4 questions=1; delete candidate
repo-behavior-shadow: PASS deterministic lookup; delete candidate
verification-route-auction: PASS scoring cases; delete candidate
adversarial: all five prototypes are deterministic; only repair-diversity changes live behavior without new persistence, execution authority, or user-visible setup
```

These results prove the candidate predicates and their rejection criteria, not
model-level repair quality. The shipped mechanism therefore remains small,
advisory, and covered by the repository's deterministic agent tests.
