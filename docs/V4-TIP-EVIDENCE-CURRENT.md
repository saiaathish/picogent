# Current V4 evidence and continuity

Current behavior checkpoint:
`547a5b1a4c8d348dae0b0c155c081f4d1b263742` (PR #685), carried by the
documentation-only candidate `e4e67c15dea4ad5fe87b3349da3bc4dc230cf0a3`.

The latest behavior slice validates catalog-bound browser screenshot admission
and transient image handoff to the next model request. A fresh task-owned
Codex observation now binds live-provider connectivity and fixed no-tool
quality to the exact documentation candidate; it does not establish an
owned-browser observation, broad hostile-filesystem TOCTOU resistance, or
release authorization.

- Exact-head matrix: `HEAD=PASS`, `tree=CLEAN`.
- Matrix summary: `PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3`.
- Matrix artifact SHA-256:
  `6be4f28d3a68322e6d6a525fc625a3f4269a46b056672c899fbc74169f828339`.
- Live-provider evidence: [V4-LIVE-PROVIDER-EVIDENCE-E4E67C1.md](V4-LIVE-PROVIDER-EVIDENCE-E4E67C1.md); connectivity and fixed quality are `PASS`.
- `rendered-platform-local`, `rendered-cross-platform`, and
  `hostile-filesystem-toctou`: `UNVERIFIED`.
- `release-authorization`: `INCONCLUSIVE`; no operator approval is recorded.

The release audit and operator gate remain in
[V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md) and
[V4-OPERATOR-RELEASE-CHECKLIST.md](V4-OPERATOR-RELEASE-CHECKLIST.md).
Historical live/rendered packets remain separate and are not unioned into a
synthetic release-authorizing result. Any later non-documentation behavior
change requires fresh evidence.
