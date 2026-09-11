# v4 live-provider evidence at behavior tip `9c1ca2e`

Status: a fresh behavior-bound matrix observation records `PASS` for direct
live-provider connectivity and the bounded `fixed-no-tool-v1` quality campaign.
The observation is self-reported at the artifact level and does not authorize a
release or claim v4 completion.

This packet belongs to the broader runtime-boundary issue
[#453](https://github.com/saiaathishkarthik/picogent/issues/453). The provider
artifacts were observed at the clean behavior tip from PR #661. The current
documentation candidate is a docs-only descendant, so the matrix uses the
documented `--behavior-sha` continuity rule.

## Provenance

| Field | Value |
| --- | --- |
| Behavior SHA | `9c1ca2e4df5af35dde5ed5e1f5cce39368e9b2fb` |
| Documentation candidate | `c3a388571b7c19e3c646ae8480ca23b275d77946` |
| Behavior provenance | `DOCS_ONLY_DESCENDANT` |
| Matrix head match / tree | `PASS` / `CLEAN` |
| Provider | `codex` |
| Campaign | `fixed-no-tool-v1` |
| Runtime | `go1.26.6 darwin/arm64` |
| Environment | `task-owned-disposable` |
| Connectivity observation | `2026-09-11T09:07:03Z` |
| Quality observation | `2026-09-11T09:13:54Z` |
| Matrix generated | `2026-09-11T09:18:30Z` |

The connectivity and quality cases ran serially from disposable Picogent homes
and empty workspaces. The retained records contain only lowercase digests and
bounded metadata. Raw provider output, stderr, credentials, and disposable
session state were removed after the digests were recorded.

## Observations

| Observation | Result digest | Latency | Tools | Workspace mutation | Verdict |
| --- | --- | ---: | --- | --- | --- |
| Connectivity (`LIVE_PROVIDER_OK`) | `05dc5c5d0202258c9b25d2c0dc47b4a232d4cff108b52629ee52da37990e41c7` | — | none observed | none observed | `PASS` |
| `exact-token` | `4472a69b5af2f7cddada501ee917a82cf088b6403ff2d61f48f3379fdd55a93a` | 4485 ms | none observed | none observed | `PASS` |
| `bounded-summary` | `90ef1c2d5988519ca3924d56d7e13fd1ce2dea582e119d0ca36736e4af468c02` | 3173 ms | none observed | none observed | `PASS` |
| `constraint-following` | `7ef6fa5dcf4e54b7f47cfb130b5d573ad9c375ca2c89877381f235681de75c06` | 2655 ms | none observed | none observed | `PASS` |

All three quality cases were below the declared 5000 ms per-case budget and
passed their canonical result checks. The separate connectivity artifact
recorded zero stderr bytes and no workspace entries. Provider identity and raw
result semantics are not independently attested.

## Retained artifacts

The secret-free records remain outside the checkout under the task-owned
directory `/private/tmp/picogent-live-refresh-evidence-c3/`:

| Artifact | SHA-256 |
| --- | --- |
| `live-provider-evidence.json` | `70fe58886e045f7c160a30568d6e67557e9ce5841411d636f0bc04ae5f6d95ac` |
| `live-provider-quality-evidence.json` | `45dbb5bcb4b5643f52efadb844ac7c5a7965027e58a57c8b4c234a7efbb33724` |
| `runtime-boundary-matrix-quality.json` | `a49c2bbb74d13f7a2053f3cbd131c7fc8aaf6dc32a295ccea2c87e2c79126293` |

The matrix reported:

```text
candidate_sha=c3a388571b7c19e3c646ae8480ca23b275d77946
behavior_sha=9c1ca2e4df5af35dde5ed5e1f5cce39368e9b2fb
behavior_provenance=DOCS_ONLY_DESCENDANT
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

Reproduce from a clean checkout at the behavior tip with task-owned disposable
provider state, then evaluate a docs-only descendant with the continuity
contract. Keep all artifacts outside the checkout:

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/absolute/path/outside/checkout/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/absolute/path/outside/checkout/live-provider-quality-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha "$(git rev-parse HEAD)" \
  --behavior-sha 9c1ca2e4df5af35dde5ed5e1f5cce39368e9b2fb \
  --out /absolute/path/outside/checkout/runtime-boundary-matrix.json
```

The continuity exception applies only to the live-provider rows and local
rendered-platform row. Any intervening code, test, workflow, build, or other
non-`docs/` change requires fresh evidence. The live-provider PASS rows do not
upgrade rendered behavior, hostile TOCTOU, recovery, or release authorization.

## Explicit limits

This record does not prove:

- provider or account identity beyond runtime selection and artifact self-report;
- streaming quality, tool-use quality, authentication refresh, or sustained sessions;
- restart, steering, undo, rendered GUI/TUI/headless behavior, or cross-platform behavior;
- arbitrary same-UID filesystem race resistance or production security; or
- v3-v4 comparative quality, release authorization, or v4 completion.

The rendered rows remain `UNVERIFIED`, the broad
`hostile-filesystem-toctou` residual remains `UNVERIFIED`, and
`release-authorization` remains `INCONCLUSIVE` without the required operator
decision.
