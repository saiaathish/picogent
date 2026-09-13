# Current V4 evidence and continuity

## Current exact behavior and evidence checkpoint

The latest non-documentation behavior tip is
4086b1f3416cde476025718139aa2c8260cc9d8e, merged by PR #723. The current
exact-head residual packet is
[V4-CURRENT-RESIDUAL-PACKET-4086B1F.md](V4-CURRENT-RESIDUAL-PACKET-4086B1F.md).

The fresh matrix was collected from a clean checkout at that exact SHA with
HEAD=PASS, tree=CLEAN, and behavior_provenance=EXACT_HEAD. Its artifact
SHA-256 is
9ee2d1a56bf6fcb92cbc1e92d4944019c480c2cc4348ac7a44aa6d1a6e77441a.
The matrix summary is UNVERIFIED=12 because this refresh supplied no
live-provider, rendered-platform, or retained hostile-runtime artifacts.
The focused evolution-store regression suite passed with low-resource
settings.

PR #723 narrowed the evolution-store coordination boundary: existing state
loads acquire the shared store lock before reading the payload, while missing
first-use state still creates nothing. It did not close the broad same-UID
filesystem TOCTOU residual. Release authorization remains unapproved with
authorized=false because no operator approval is recorded. Parent #453 remains
OPEN.

The evidence documents in this branch are documentation-only descendants of
the exact behavior candidate. They may retain this packet under the
continuity rule, but must not silently rebind its claims to a later docs
commit or combine historical live/rendered packets into a release decision.

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
