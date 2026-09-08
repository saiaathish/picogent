# V4 exact-tip evidence packet at `1ce2059`

Status: **NOT COMPLETE / unauthorized**. This packet binds the latest bounded
runtime evidence to the clean behavior candidate
`1ce2059fc352b5d32b5da3adbaeb1ecb48834db7`. It does not close
[#450](https://github.com/saiaathish/picogent/issues/450) or
[#453](https://github.com/saiaathish/picogent/issues/453), and it does not
authorize a release.

## Exact candidate

```text
repository:  github.com/saiaathish/picogent
candidate:   1ce2059fc352b5d32b5da3adbaeb1ecb48834db7
head_match:  PASS
tree:        CLEAN
provenance:  EXACT_HEAD
```

This candidate is the post-merge `main` tip after the bounded Linux same-UID
final-path replacement lane. Earlier evidence tied to non-documentation
ancestors is not silently projected onto this candidate.

## Current matrix

The retained full matrix at
`/private/tmp/picogent-evidence-1ce2059/runtime-boundary-matrix-full.json` has
SHA-256
`c0e56a26407c19828a9e1a76e6d8a8e229cdaf6ffc1c7b0b7056ba52ea7ed6c7` and
records **PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1**:

| Claim | Verdict | Evidence boundary |
| --- | --- | --- |
| `hostile-child-env-sanitization` | `PASS` | Deterministic hostile child-environment controls. |
| `hostile-filesystem-deterministic` | `PASS` | Named deterministic securefile/procenv/workspace families. |
| `hostile-filesystem-toctou` | `UNVERIFIED` | Broad arbitrary same-UID pathname races remain open. |
| `hostile-parent-swap-confinement` | `PASS` | Bounded Darwin/Linux parent-swap confinement. |
| `live-provider-connectivity` | `PASS` | One bounded real-provider fixed prompt. |
| `live-provider-quality` | `PASS` | Three-case fixed no-tool campaign; self-reported result semantics. |
| `rendered-platform-local` | `PASS` | Darwin task-owned rendered recovery observation. |
| `rendered-cross-platform` | `PASS` | Darwin/Linux/Windows aggregate at this exact candidate. |
| `rendered-long-horizon-local` | `PASS` | Deterministic local long-horizon fixture. |
| `rendered-recovery-undo-reload` | `PASS` | Deterministic recovery contract. |
| `restart-steer-undo-recovery` | `PASS` | Deterministic restart, steering, and undo recovery. |
| `release-authorization` | `INCONCLUSIVE` | Human operator approval is absent. |

The live-provider and rendered records contain the detailed provenance and
limits:

- [live-provider evidence](V4-LIVE-PROVIDER-EVIDENCE-1CE2059.md)
- [rendered cross-platform evidence](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-1CE2059.md)
- [Linux final-path hostile evidence](V4-HOSTILE-FINAL-PATH-LINUX.md)

## Hosted validation

The exact candidate passed the post-merge hosted checks:

- [CI run 34192467231](https://github.com/saiaathish/picogent/actions/runs/34192467231)
- [release-artifacts run 34192467240](https://github.com/saiaathish/picogent/actions/runs/34192467240)
- [Linux rendered run 34222585513](https://github.com/saiaathish/picogent/actions/runs/34222585513)
- [Windows rendered run 34222591261](https://github.com/saiaathish/picogent/actions/runs/34222591261)

These runs provide hosted CI and owned-browser evidence for their named
surfaces. They do not turn the residual TOCTOU row into `PASS` or provide the
human release decision.

## Release boundary

The packet is intentionally not a completion claim:

- broad `hostile-filesystem-toctou` is still `UNVERIFIED`;
- the live-provider records are bounded and self-reported, not independent
  provider/account attestation;
- the rendered aggregate proves the named recovery fixture, not every GUI/TUI/
  headless or arbitrary repository journey; and
- `release-authorization` stays `INCONCLUSIVE` until the human operator records
  the required decision.

The host-local evidence root is disposable operator evidence, not a repository
artifact: `/private/tmp/picogent-evidence-1ce2059/`.
