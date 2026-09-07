# v4 live-provider evidence at tip afc9fc6

Status: exact-head matrix `PASS` for live-provider connectivity and the bounded
`fixed-no-tool-v1` quality campaign at source
`afc9fc6f85c23976d5a356757f48f1c5e4d50f4b`. This checkpoint belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

This is a bounded self-reported observation. It is not independent provider or
account attestation and does not prove streaming, tool use, recovery,
cross-platform rendering, hostile-runtime safety, or release authorization.

## Provenance

```text
repository:  github.com/saiaathish/picogent
source:      afc9fc6f85c23976d5a356757f48f1c5e4d50f4b
source_ref:  origin/main (fetched and matched before observation)
provider:    codex
model:       gpt-5.6-luna
campaign:    connectivity + fixed-no-tool-v1
runtime:     go1.26.6 darwin/arm64
environment: task-owned-disposable
observed:    2026-09-06T23:58:16Z (connectivity)
             2026-09-06T23:58:30Z (quality)
```

The source worktree was clean and matched `origin/main`. The binary was built
from that worktree and run with `PICOGENT_PROVIDER=codex`,
`PICOGENT_MODEL=gpt-5.6-luna`, and `PICOGENT_ROUTER=off`. Each observation used
a disposable `PICOGENT_HOME` and workspace plus a task-owned copy of Codex
credentials. The copy, raw responses, stderr, and provider session state were
removed after digest generation.

The first campaign attempt was rejected because `constraint-following` took
5017 ms against the 5000 ms quality budget. A fresh bounded rerun produced the
accepted artifacts below; its maximum quality latency was exactly 5000 ms.

## Collection command pattern

```sh
go build -trimpath \
  -o /private/tmp/picogent-evidence-afc9fc6/picogent \
  ./cmd/picogent

PICOGENT_HOME="$disposable_home" \
PICOGENT_CODEX_HOME="$disposable_codex_home" \
CODEX_HOME="$disposable_codex_home" \
PICOGENT_PROVIDER=codex \
PICOGENT_MODEL=gpt-5.6-luna \
PICOGENT_ROUTER=off \
/private/tmp/picogent-evidence-afc9fc6/picogent run \
  --dir "$disposable_workspace" --yes "$fixed_prompt"
```

The second command was run once per canonical connectivity / quality prompt.
Only prompt and normalized-result SHA-256 digests, latency, no-tool and
no-mutation assertions, timestamps, and verdicts were retained.

Host-local artifacts outside the checkout:

```text
/private/tmp/picogent-evidence-afc9fc6/live-provider-evidence.json
/private/tmp/picogent-evidence-afc9fc6/live-provider-quality-evidence.json
/private/tmp/picogent-evidence-afc9fc6/runtime-boundary-matrix.json
```

Artifact SHA-256:

```text
connectivity ce4a81ba726ebe1efb8679cfe1d694bad16a4841c4dd66ccafdc057c1f171c53
quality      1c8d93beae219b8667c59c982b5abc90464d5a3d1095b94a38171f7b232f3d4b
matrix       aed21c01a385abeef5bc160b96f256c2fa96fc69eada5171c5280478582bb86a
```

## Observations

| Observation | Result SHA-256 | Latency | Verdict |
| --- | --- | ---: | --- |
| connectivity | `c3e38e6256a68d4c57f7644d794f45532d71799556c494c57ec567973e8ea8c7` | 3486 ms | `PASS` |
| `exact-token` | `46a0a304898ade94da30c281d5f6fa5d61ce783cfcea6f4735b19671bea364b0` | 4664 ms | `PASS` |
| `bounded-summary` | `990b5257e4a65fd9cf4710c89c32c0eaf0825ee547cf2b64e2702228eae5af04` | 4610 ms | `PASS` |
| `constraint-following` | `c07d57c5cdfac32af7d21cb4a96ab1d394a125240387ff916b93d91f51c3ea65` | 5000 ms | `PASS` |

All accepted quality cases recorded `tools_used=false` and
`mutation_observed=false`.

## Matrix validation

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/private/tmp/picogent-evidence-afc9fc6/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/private/tmp/picogent-evidence-afc9fc6/live-provider-quality-evidence.json \
PICOGENT_RENDERED_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_PLATFORM_ARTIFACT=/private/tmp/picogent-evidence-afc9fc6/rendered-platform-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha afc9fc6f85c23976d5a356757f48f1c5e4d50f4b \
  --out /private/tmp/picogent-evidence-afc9fc6/runtime-boundary-matrix.json
```

The matrix recorded `HEAD=PASS`, `tree=CLEAN`, `PASS:8`,
`INCONCLUSIVE:1`, and `UNVERIFIED:2`.

Explicitly unchanged boundaries:

- `rendered-cross-platform=UNVERIFIED`
- `hostile-filesystem-toctou=UNVERIFIED`
- `release-authorization=INCONCLUSIVE`

This artifact is bound to the pre-documentation source SHA above and must not
be silently rebound to a later merge SHA.
