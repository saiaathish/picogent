# Current V4 evidence and continuity

Stable entrypoint for the current main behavior candidate
`a80b9fb73d3d74a2c9787fdd7c74cd9ba2dd65cf`:

- Bounded goal-state evidence from the last non-documentation candidate
  `eccebd293e58ec48c6553e9228ff4b201a74c77a`:
  [V4-GOAL-STATE-EVIDENCE.md](V4-GOAL-STATE-EVIDENCE.md).
- Current release-audit posture:
  [V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md).
- Current operator gate:
  [V4-OPERATOR-RELEASE-CHECKLIST.md](V4-OPERATOR-RELEASE-CHECKLIST.md).

The current main post-merge CI run `34543854046` and release-artifacts run
`34543854052` passed. No fresh live-provider or rendered-platform observation
is bound to the current behavior candidate after the goal-state hardening
changes, so those rows remain `UNVERIFIED`. The older live-provider continuity
and rendered cross-platform packets remain historical and separate; they must
not be unioned into a synthetic release-authorizing result. Broad hostile
filesystem TOCTOU remains `UNVERIFIED`, and release authorization remains
`INCONCLUSIVE` without an operator decision. A later non-documentation
behavior change requires fresh evidence.
