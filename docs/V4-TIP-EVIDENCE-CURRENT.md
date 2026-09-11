# Current V4 evidence and continuity

Current documentation tip:
592b07a633d9683354c4abeee09b767bc071ab35.

The behavior-bound evidence baseline is
1865a4c00356ac9b871b35b3a08f4e983f2af0e1. The current tip is proven to be a
documentation-only descendant of that behavior candidate.

- Bounded goal-state evidence from the last non-documentation candidate:
  [V4-GOAL-STATE-EVIDENCE.md](V4-GOAL-STATE-EVIDENCE.md).
- Fresh live-provider connectivity and quality evidence:
  [V4-LIVE-PROVIDER-QUALITY-EVIDENCE-1865A4C.md](V4-LIVE-PROVIDER-QUALITY-EVIDENCE-1865A4C.md).
- Exact-current three-platform rendered recovery evidence:
  [V4-RENDERED-CROSS-PLATFORM-EVIDENCE-592B07A.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-592B07A.md).
- Current release-audit posture:
  [V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md).
- Current operator gate:
  [V4-OPERATOR-RELEASE-CHECKLIST.md](V4-OPERATOR-RELEASE-CHECKLIST.md).

The current runtime-boundary matrix is PASS 10 / INCONCLUSIVE 1 /
UNVERIFIED 1: live-provider connectivity and fixed quality are PASS,
rendered local and cross-platform recovery are PASS, and broad hostile
filesystem TOCTOU remains explicitly UNVERIFIED. Release authorization
remains INCONCLUSIVE because no human operator decision has been recorded.
Historical live/rendered packets remain separate and are not unioned into a
synthetic release-authorizing result. Any later non-documentation behavior
change requires fresh evidence.
