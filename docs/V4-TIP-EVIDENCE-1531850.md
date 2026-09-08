# V4 exact-tip evidence packet at `1531850`

Status: **NOT COMPLETE / unauthorized**. This packet binds the bounded
runtime-boundary matrix to the clean post-merge behavior candidate
`1531850d80485e441bae58369595cb15a57e4c99`. It does not close
[#450](https://github.com/saiaathishkarthik/picogent/issues/450) or
[#453](https://github.com/saiaathishkarthik/picogent/issues/453), and it does
not authorize a release.

## Exact candidate

```text
repository:          github.com/saiaathish/picogent
candidate:           1531850d80485e441bae58369595cb15a57e4c99
behavior:            1531850d80485e441bae58369595cb15a57e4c99
head_match:          PASS
tree:                CLEAN
behavior_provenance: EXACT_HEAD
generated_at:        2026-09-08T22:14:30Z
```

This is a fresh exact-head collection after PR #581 merged. A continuity
attempt using behavior `97a3ec5` was correctly rejected because PR #581 touched
`README.md`; the continuity contract accepts only descendants whose intervening
paths are under `docs/`. This packet therefore does not project the earlier
`97a3ec5` matrix onto the merge tip.

## Retained matrix

The exact-head matrix was retained outside the checkout at
`/private/tmp/picogent-candidate-06e06a1/runtime-boundary-matrix-1531850.json`.
Its SHA-256 is
`d261af32e1d547b9c97243f4d4b3667c4ba6a99981770ff80ea4dd0915c3957c`.

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
  --candidate-sha 1531850d80485e441bae58369595cb15a57e4c99 \
  --out /private/tmp/picogent-candidate-06e06a1/runtime-boundary-matrix-1531850.json
```

## Hosted and local boundary

PR #581 passed hosted macOS, Ubuntu, Windows, security, release-evidence, and
production-artifact checks before merging. Those checks validate the evidence
documentation changes; they do not provide current-tip live-provider or
rendered-browser observations.

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

The `97a3ec5` packet is a historical pre-merge checkpoint. The older `1ce2059`
and `3f5543d` live/rendered packets are also historical. Future documentation-
only descendants may use `--behavior-sha 1531850d80485e441bae58369595cb15a57e4c99`
only when every intervening commit path remains under `docs/`.
