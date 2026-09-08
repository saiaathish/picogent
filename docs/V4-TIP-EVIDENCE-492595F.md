# V4 exact-tip evidence packet at `492595f`

Status: **NOT COMPLETE / unauthorized**. This packet binds the bounded
runtime-boundary matrix to the clean post-merge behavior candidate
`492595fb69bb02a91665cb34998a9d7c486ead72`. It does not close
[#450](https://github.com/saiaathishkarthik/picogent/issues/450) or
[#453](https://github.com/saiaathishkarthik/picogent/issues/453), and it does
not authorize a release.

## Exact candidate

```text
repository:          github.com/saiaathishkarthik/picogent
candidate:           492595fb69bb02a91665cb34998a9d7c486ead72
behavior:            492595fb69bb02a91665cb34998a9d7c486ead72
head_match:          PASS
tree:                CLEAN
behavior_provenance: EXACT_HEAD
generated_at:        2026-09-08T22:44:30Z
```

This is a fresh exact-head collection after docs-only PR #583 merged. The
prior `1531850` packet is retained as historical evidence; this packet does
not project its live-provider or rendered-platform boundaries onto the new
candidate.

## Retained matrix

The exact-head matrix was retained outside the checkout at
`/private/tmp/picogent-candidate-06e06a1/runtime-boundary-matrix-492595f.json`.
Its SHA-256 is
`a59a2d485aff8156fa31cc99f70ec8f260562f238999fd1e96b15402f94acbfa`.

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
  --candidate-sha 492595fb69bb02a91665cb34998a9d7c486ead72 \
  --out /private/tmp/picogent-candidate-06e06a1/runtime-boundary-matrix-492595f.json
```

## Hosted and local boundary

PR #583 passed hosted macOS, Ubuntu, Windows, security, release-evidence, and
production-artifact checks before merging. Those checks validate the
documentation-only evidence refresh; they do not provide current-tip
live-provider or rendered-browser observations.

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

The `1531850` packet is a historical docs-tip checkpoint. The older `97a3ec5`,
`1ce2059`, and `3f5543d` live/rendered packets are also historical. Future
documentation-only descendants may use `--behavior-sha 492595fb69bb02a91665cb34998a9d7c486ead72`
only when every intervening commit path remains under `docs/`.
