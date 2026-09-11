# Current V4 evidence and continuity

Stable entrypoint for the latest exact behavior candidate
`1865a4c00356ac9b871b35b3a08f4e983f2af0e1`:

- Bounded goal-state evidence from the last non-documentation candidate
  `eccebd293e58ec48c6553e9228ff4b201a74c77a`:
  [V4-GOAL-STATE-EVIDENCE.md](V4-GOAL-STATE-EVIDENCE.md).
- Fresh exact-head live-provider connectivity and quality evidence:
  [V4-LIVE-PROVIDER-QUALITY-EVIDENCE-1865A4C.md](V4-LIVE-PROVIDER-QUALITY-EVIDENCE-1865A4C.md).
- Fresh task-owned Darwin rendered recovery evidence:
  [V4-RENDERED-RECOVERY-EVIDENCE-1865A4C.md](V4-RENDERED-RECOVERY-EVIDENCE-1865A4C.md).
- Current release-audit posture:
  [V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md).
- Current operator gate:
  [V4-OPERATOR-RELEASE-CHECKLIST.md](V4-OPERATOR-RELEASE-CHECKLIST.md).

The exact-head packet records `live-provider-connectivity=PASS`,
`live-provider-quality=PASS`, and the new Darwin local rendered observation
records `rendered-platform-local=PASS`. The docs-tip continuity matrix summary
is PASS 9 / INCONCLUSIVE 1 / UNVERIFIED 2. Cross-platform rendered behavior
and broad hostile filesystem TOCTOU remain `UNVERIFIED`, and release
authorization remains `INCONCLUSIVE` without an operator decision. The older live-provider
continuity and rendered cross-platform packets remain historical and separate;
they must not be unioned into a synthetic release-authorizing result. The
packet was collected before this documentation-only update, so later
documentation-only descendants must use the runtime-boundary continuity
contract; any later non-documentation behavior change requires fresh evidence.
