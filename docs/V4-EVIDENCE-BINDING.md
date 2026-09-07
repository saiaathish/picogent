# v4 verification evidence binding

Status: bounded implementation on the `codex/v4-evidence-binding` branch.

## Contract

Durable verification records may carry a `workspace.Observation` containing
the canonical workspace identity and bounded observations of the paths covered
by the check. The observation stores metadata and SHA-256 digests, never file
contents. Only the latest verification retains its observation so task state
stays bounded as checks accumulate.

The agent captures the requested paths immediately before and after a
verification tool runs. A mutation, replacement, truncated capture, oversized
file, unsafe path, missing path set, or other unknown observation makes a PASS
inconclusive. A tool can still report its raw result to the current user, but
an unusable PASS is never durable completion evidence.

## Lifecycle

1. A passing verification is stored with its observation and the task's
   current change sequence.
2. Loading a task after restart re-captures the observed paths. A changed or
   unknown comparison converts the persisted PASS to `INCONCLUSIVE` and saves
   that invalidation with the task's compare-and-swap revision.
3. Before an active goal is completed, the agent re-captures the latest
   observation once more. This catches edits that happen after verification
   but before the completion result is published.
4. A conflicting task revision prevents the stale candidate from becoming the
   agent's in-memory or published task state.

## Deliberate limits

This is a bounded evidence boundary, not an atomic whole-workspace snapshot.
It cannot detect an external A-to-B-to-A rewrite when the final digest and
identity are indistinguishable, and an empty target set is deliberately not
fresh evidence. It does not add a watcher, recursive tree hash, criterion-level
authority, hypothesis graph, or diagnosis engine.

## Runtime artifact behavior SHA

The runtime-boundary matrix has a separate, narrow continuity rule for
externally retained live-provider and local rendered-platform artifacts. The
matrix artifact and release candidate remain bound to clean exact `HEAD`.
When `--behavior-sha` differs from `--candidate-sha`, Git must prove that the
behavior SHA is an ancestor and that every intervening commit touches only
paths under `docs/`; a later revert does not hide a non-docs change. The selected live/local-rendered artifact loaders
then validate their embedded `candidate_sha` against that proven behavior SHA.

This is acceptance of unchanged behavior provenance, not rebinding: the
external artifact keeps its original SHA and digest. Wrong artifact SHAs,
dirty trees, non-docs descendants, missing ancestry/diff provenance, malformed
records, and unsafe artifact paths reject. Cross-platform rendered evidence,
hostile-filesystem TOCTOU, production artifacts, verification manifests,
attestations, and release authorization do not inherit this allowance.
