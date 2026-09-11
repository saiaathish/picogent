# Current V4 evidence and continuity

Stable entrypoint for the current main behavior candidate
`216600450a68c4603f8e2460279688cc56f08c0b`:

- Bounded goal-state evidence from the last non-documentation candidate
  `eccebd293e58ec48c6553e9228ff4b201a74c77a`:
  [V4-GOAL-STATE-EVIDENCE.md](V4-GOAL-STATE-EVIDENCE.md).
- Fresh exact-head live-provider connectivity evidence:
  [V4-LIVE-PROVIDER-EVIDENCE-2166004.md](V4-LIVE-PROVIDER-EVIDENCE-2166004.md).
- Current release-audit posture:
  [V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md).
- Current operator gate:
  [V4-OPERATOR-RELEASE-CHECKLIST.md](V4-OPERATOR-RELEASE-CHECKLIST.md).

The current main post-merge CI run `34546564353` and release-artifacts run
`34546564358` passed. The fresh connectivity packet records
`live-provider-connectivity=PASS`; no fresh live-provider quality or rendered
platform observation is bound to the current behavior candidate after the
goal-state hardening changes, so those rows remain `UNVERIFIED`. The older
live-provider continuity and rendered cross-platform packets remain historical
and separate; they must not be unioned into a synthetic release-authorizing
result. Broad hostile filesystem TOCTOU remains `UNVERIFIED`, and release
authorization remains `INCONCLUSIVE` without an operator decision. A later
non-documentation behavior change requires fresh evidence.
