# v4 live-provider evidence at exact tip `b7b5c5c`

Status: the exact-head matrix records `PASS` for live-provider connectivity
and the bounded `fixed-no-tool-v1` quality campaign at the merged setup
contract. This is a bounded, self-reported observation; it does not authorize
a release or claim v4 completion.

This checkpoint belongs to [#619](https://github.com/saiaathishkarthik/picogent/issues/619)
under runtime-boundary parent
[#453](https://github.com/saiaathishkarthik/picogent/issues/453).

## Provenance

| Field | Value |
| --- | --- |
| Behavior / candidate SHA | `b7b5c5c3e98f2caaf1c0d951740bdd7046f755f5` |
| Behavior provenance | `EXACT_HEAD` |
| Head match / tree | `PASS` / `CLEAN` |
| Provider / model | `codex` / `gpt-5.6-luna` |
| Runtime | `go1.26.6 darwin/arm64` |
| Environment | `task-owned-disposable` |
| Matrix generated | `2026-09-10T09:04:57Z` |

The probes ran from the clean exact-tip checkout with
`PICOGENT_PROVIDER=codex`, `PICOGENT_MODEL=gpt-5.6-luna`, and
`PICOGENT_ROUTER=off`. Connectivity and each quality case ran in a fresh
process with disposable Picogent state and workspace. The temporary
credential was read from the existing local Codex login but was not copied
into the checkout or retained in the evidence directory. Raw provider output,
stderr, and disposable state were removed after digest generation.

## Observations

| Observation | Result SHA-256 | Command wall time | Stderr | Workspace entries | Verdict |
| --- | --- | ---: | ---: | ---: | --- |
| Connectivity (`LIVE_PROVIDER_OK`) | `05dc5c5d0202258c9b25d2c0dc47b4a232d4cff108b52629ee52da37990e41c7` | 2970 ms | 0 bytes | 0 | `PASS` |
| `exact-token` | `4472a69b5af2f7cddada501ee917a82cf088b6403ff2d61f48f3379fdd55a93a` | 2940 ms | 0 bytes | 0 | `PASS` |
| `bounded-summary` | `3e544763f99192e1d1580a270e93e132a4bb5627fecdbe32cb6546f921f8afcc` | 3220 ms | 0 bytes | 0 | `PASS` |
| `constraint-following` | `7ef6fa5dcf4e54b7f47cfb130b5d573ad9c375ca2c89877381f235681de75c06` | 3160 ms | 0 bytes | 0 | `PASS` |

The three quality cases stayed below the declared 5000 ms per-case budget.
The exact-token and three-word cases matched their canonical contracts. The
summary case produced one non-empty line ending in sentence punctuation.
`tools_used=false` and `mutation_observed=false` are explicit operator
assertions supported by the no-tool prompts, zero workspace entries, and zero
captured stderr bytes; provider identity and raw result semantics are not
independently attested.

## Retained artifacts

The secret-free artifacts remain outside the checkout under the task-owned
directory `/private/tmp/picogent-live-619.R6pSMq/`:

| Artifact | SHA-256 |
| --- | --- |
| `live-provider-evidence.json` | `ad68608969edf395de89a6f11ebe4396680d6c92c2101d4cd771906692832d09` |
| `live-provider-quality-evidence.json` | `b114b76f87b5983da0fedde5db75eb8473defab02637b82755b4e9d2cf72a9d2` |
| `runtime-boundary-matrix.json` | `5be231cd13d31915753c1743365bbe0582f47accd3e210f6f493b8f4264d074f` |

The matrix recorded:

```text
candidate_sha=b7b5c5c3e98f2caaf1c0d951740bdd7046f755f5
behavior_provenance=EXACT_HEAD
head_match=PASS  tree=CLEAN
summary=PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3
live-provider-connectivity=PASS
live-provider-quality=PASS
rendered-platform-local=UNVERIFIED
rendered-cross-platform=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

## Reproduction boundary

Reproduce from a clean checkout at the exact candidate SHA with a disposable
provider home and workspace. Keep all referenced artifacts outside the
checkout:

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/absolute/path/outside/checkout/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/absolute/path/outside/checkout/live-provider-quality-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha b7b5c5c3e98f2caaf1c0d951740bdd7046f755f5 \
  --out /absolute/path/outside/checkout/runtime-boundary-matrix.json
```

The matrix flags are opt-in and independent: connectivity evidence does not
upgrade quality, and either live-provider row does not upgrade rendered,
hostile-runtime, recovery, or release-authorization claims.

## Explicit limits

This record does not prove:

- provider or account identity beyond runtime selection and artifact
  self-report;
- streaming, tool-use quality, authentication refresh, or sustained sessions;
- rendered local or cross-platform browser behavior;
- arbitrary same-UID filesystem race resistance; or
- release authorization or v4 completion.

The rendered rows remain `UNVERIFIED`, the broad
`hostile-filesystem-toctou` residual remains `UNVERIFIED`, and
`release-authorization` remains `INCONCLUSIVE` without the required operator
decision.
