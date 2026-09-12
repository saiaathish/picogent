# Current V4 evidence and continuity

## Current documentation tip

The current clean main documentation tip is
e26a57ef0e5d4c1ade67d9a87a74cd42ebbeca47, merged by PR #715. The current
residual posture is recorded in
V4-CURRENT-RESIDUAL-PACKET-E26A57E.md.
PR #715 is a documentation-only descendant of behavior
6914a42abddb1a789a9a655abeb4db61d9e9956c; it preserves the direct
long-horizon observation without rebinding older runtime packets.

The broad same-UID filesystem TOCTOU row remains UNVERIFIED. Release
authorization remains INCONCLUSIVE with authorized=false because no operator
approval is recorded. Parent #453 remains OPEN.

## Superseded exact behavior evidence checkpoint
`3535bda907dbe8cc44ca43d5563e4e607a96d266` (PR #689 merge), with the fresh
#690 rendered packet documented in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3535BDA.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-3535BDA.md).

The exact candidate has task-owned Darwin, Linux, and Windows rendered
recovery observations plus bounded live-provider artifacts. This is an
evidence checkpoint, not a release approval: provider identity and raw result
semantics remain self-reported, broad hostile-filesystem TOCTOU is not proved,
and no operator authorization is recorded.

- Exact-head matrix: `HEAD=PASS`, `tree=CLEAN`, `behavior_provenance=EXACT_HEAD`.
- Matrix summary: `PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1`.
- Matrix artifact SHA-256:
  `f9576b69a41c8d0eb3e036dc8d82b7057f1eb7c842d9ffd5086916408fada2cc`.
- `live-provider-connectivity`, `live-provider-quality`,
  `rendered-platform-local`, and `rendered-cross-platform`: `PASS` under
  their bounded artifact contracts.
- `hostile-filesystem-toctou`: `UNVERIFIED`.
- `release-authorization`: `INCONCLUSIVE`; no operator approval is recorded.

The release audit and operator gate remain in
[V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md) and
[V4-OPERATOR-RELEASE-CHECKLIST.md](V4-OPERATOR-RELEASE-CHECKLIST.md).
Historical live/rendered packets remain separate and are not unioned into a
synthetic release-authorizing result. Any later non-documentation behavior
change requires fresh evidence, and later documentation-only descendants must
not silently rebind this exact-candidate packet.
