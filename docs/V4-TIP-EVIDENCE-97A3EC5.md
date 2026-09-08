# V4 exact-tip evidence packet at `97a3ec5`

Status: **NOT COMPLETE / unauthorized**. This packet binds the bounded
runtime-boundary matrix to the clean behavior candidate
`97a3ec5951137a353fcb42276c701040e1e716c1`. It does not close
[#450](https://github.com/saiaathishkarthik/picogent/issues/450) or
[#453](https://github.com/saiaathishkarthik/picogent/issues/453), and it does
not authorize a release.

## Exact candidate

```text
repository:          github.com/saiaathish/picogent
candidate:           97a3ec5951137a353fcb42276c701040e1e716c1
behavior:            97a3ec5951137a353fcb42276c701040e1e716c1
head_match:          PASS
tree:                CLEAN
behavior_provenance: EXACT_HEAD
generated_at:        2026-09-08T21:51:50Z
```

The candidate is the post-merge `main` tip after PR #578's legacy descendant
process-tree timeout coverage and PR #579's GUI auth-status single-flight
probe fix. Their non-documentation changes invalidate older tip-bound runtime
observations; those records remain historical and are not silently rebound.

## Retained matrix

The exact-head matrix was retained outside the checkout at
`/private/tmp/picogent-candidate-06e06a1/runtime-boundary-matrix-97a3ec5.json`.
Its SHA-256 is
`f244d37976965874d25a9465c071cdc6ff63caf26a411182ccf339bd003dddc3`.

The report records **PASS 6 / INCONCLUSIVE 1 / UNVERIFIED 5**:

| Claim | Verdict | Evidence boundary |
| --- | --- | --- |
| `hostile-child-env-sanitization` | `PASS` | Bounded child-environment sanitization evidence is documented; arbitrary same-UID TOCTOU remains outside this claim. |
| `hostile-filesystem-deterministic` | `PASS` | Bounded deterministic hostile-runtime evidence is documented; arbitrary same-UID filesystem TOCTOU remains outside this claim. |
| `hostile-filesystem-toctou` | `UNVERIFIED` | Broad same-UID pathname races remain unverified; bounded parent-swap confinement is a separate claim. |
| `hostile-parent-swap-confinement` | `PASS` | Bounded Darwin and Linux parent-swap confinement evidence is documented. |
| `live-provider-connectivity` | `UNVERIFIED` | No current-tip live-provider connectivity artifact was supplied. |
| `live-provider-quality` | `UNVERIFIED` | No current-tip live-provider quality artifact was supplied. |
| `release-authorization` | `INCONCLUSIVE` | Human approval is absent, and live/rendered gaps plus the broad TOCTOU residual remain. |
| `rendered-cross-platform` | `UNVERIFIED` | No current-tip three-platform rendered aggregate was supplied. |
| `rendered-long-horizon-local` | `PASS` | The deterministic local rendered long-horizon evidence contract is retained; unsupported platform journeys remain outside this claim. |
| `rendered-platform-local` | `UNVERIFIED` | No current-tip rendered-platform artifact was supplied. |
| `rendered-recovery-undo-reload` | `PASS` | Deterministic allow-to-undo-to-reload API-boundary evidence is retained; browser DOM and live-provider behavior remain outside this claim. |
| `restart-steer-undo-recovery` | `PASS` | Deterministic restart, steering, and undo contracts are documented; live-provider recovery remains outside this claim. |

No claim was upgraded from artifact presence alone. The matrix was generated
from a clean checkout at the exact candidate with:

```sh
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha 97a3ec5951137a353fcb42276c701040e1e716c1 \
  --out /private/tmp/picogent-candidate-06e06a1/runtime-boundary-matrix-97a3ec5.json
```

## Hosted and local boundary

PR #578 and PR #579 passed their required hosted macOS, Ubuntu, Windows,
security, production-artifact, and release-evidence checks before their merges.
Those checks validate the named changes; they do not provide current-tip live
provider or rendered-browser observations.

The current matrix intentionally supplies no live-provider or rendered-platform
artifacts. Therefore both live-provider rows and both rendered-platform rows
remain `UNVERIFIED`. The broad hostile filesystem TOCTOU row also remains
`UNVERIFIED`; deterministic fixtures and bounded parent-swap tests do not prove
arbitrary same-UID races.

## Release boundary

This packet is not a completion claim:

- `live-provider-connectivity` and `live-provider-quality` remain `UNVERIFIED`;
- `rendered-platform-local` and `rendered-cross-platform` remain `UNVERIFIED`;
- broad `hostile-filesystem-toctou` remains `UNVERIFIED`; and
- `release-authorization` remains `INCONCLUSIVE` without the required human
  decision and complete release evidence.

The older `1ce2059` and `3f5543d` live/rendered packets are historical. A later
non-documentation change requires a fresh observation at the new behavior SHA;
this packet is that fail-closed matrix refresh, not a release approval.
