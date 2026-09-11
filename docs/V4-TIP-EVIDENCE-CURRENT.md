# Current V4 evidence and continuity

Current behavior checkpoint:
`547a5b1a4c8d348dae0b0c155c081f4d1b263742` (PR #685).

The latest behavior slice validates catalog-bound browser screenshot admission
and transient image handoff to the next model request. It does not establish
an owned-browser observation, live-provider quality, broad hostile-filesystem
TOCTOU resistance, or release authorization.

- Exact-head matrix: `HEAD=PASS`, `tree=CLEAN`.
- Matrix summary: `PASS 6 / INCONCLUSIVE 1 / UNVERIFIED 5`.
- Matrix artifact SHA-256:
  `8ce4dde2c26a797bdfd4d1bd56b61e713334383b9bdef44c7b8acbc90ce778b2`.
- `live-provider-connectivity`, `live-provider-quality`,
  `rendered-platform-local`, `rendered-cross-platform`, and
  `hostile-filesystem-toctou`: `UNVERIFIED`.
- `release-authorization`: `INCONCLUSIVE`; no operator approval is recorded.

The release audit and operator gate remain in
[V4-FINAL-RELEASE-AUDIT.md](V4-FINAL-RELEASE-AUDIT.md) and
[V4-OPERATOR-RELEASE-CHECKLIST.md](V4-OPERATOR-RELEASE-CHECKLIST.md).
Historical live/rendered packets remain separate and are not unioned into a
synthetic release-authorizing result. Any later non-documentation behavior
change requires fresh evidence.
