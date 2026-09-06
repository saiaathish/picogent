# v4 live-provider evidence at tip 28cc214

Status: exact-head matrix `PASS` for live-provider connectivity and the bounded
`fixed-no-tool-v1` quality campaign at source
`28cc214834f4d46dff555b2e60c83cd526827b31`. This checkpoint belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

This is a bounded self-reported observation. It is not independent provider or
account attestation and does not prove streaming, tool use, recovery,
cross-platform rendering, hostile-runtime safety, or release authorization.

## Provenance

```text
repository:  github.com/saiaathish/picogent
source:      28cc214834f4d46dff555b2e60c83cd526827b31
source_ref:  origin/main (fetched and matched before observation)
provider:    codex
model:       gpt-5.6-luna
campaign:    connectivity + fixed-no-tool-v1
runtime:     go1.26.6 darwin/arm64
environment: task-owned-disposable
observed:    2026-09-06T13:29:59Z (connectivity)
             2026-09-06T13:30:06Z (quality)
```

The source worktree was clean and matched `origin/main`. The binary was built
from that worktree and run with `PICOGENT_PROVIDER=codex`,
`PICOGENT_MODEL=gpt-5.6-luna`, and `PICOGENT_ROUTER=off`. Each observation used
a disposable `PICOGENT_HOME` and workspace plus a task-owned copy of Codex
credentials. The copy, raw responses, stderr, and provider session state were
removed after digest generation.

Host-local artifacts outside the checkout:

```text
/private/tmp/picogent-evidence-28cc214/live-provider-evidence.json
/private/tmp/picogent-evidence-28cc214/live-provider-quality-evidence.json
/private/tmp/picogent-evidence-28cc214/runtime-boundary-matrix.json
```

Artifact SHA-256:

```text
connectivity dc53f792803e6b52cf6914200eabeda361c8f2397b0bfedb387192f29d970590
quality      0ea0f864b7ac50c264fc0b2af314363084d127673ecd4b01553f2b3703bb0993
matrix       acb08bd8bc8414e2f97f62310565ff66803c06daad455d9283ab4319a0da8f75
```

## Observations

Connectivity returned the fixed expected token without tools or workspace
mutation.

| Observation | Result SHA-256 | Latency | Verdict |
| --- | --- | ---: | --- |
| connectivity | `c3e38e6256a68d4c57f7644d794f45532d71799556c494c57ec567973e8ea8c7` | 3423 ms | `PASS` |
| `exact-token` | `46a0a304898ade94da30c281d5f6fa5d61ce783cfcea6f4735b19671bea364b0` | 2393 ms | `PASS` |
| `bounded-summary` | `2a87b2b7282e42633c3df7fe7d552abc486655e616ad5ed95810a97f917cc1db` | 2388 ms | `PASS` |
| `constraint-following` | `c07d57c5cdfac32af7d21cb4a96ab1d394a125240387ff916b93d91f51c3ea65` | 2484 ms | `PASS` |

All quality cases stayed below the declared 5000 ms budget and recorded
`tools_used=false` and `mutation_observed=false`.

## Matrix validation

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/private/tmp/picogent-evidence-28cc214/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/private/tmp/picogent-evidence-28cc214/live-provider-quality-evidence.json \
PICOGENT_RENDERED_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_PLATFORM_ARTIFACT=/private/tmp/picogent-evidence-28cc214/rendered-platform-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha 28cc214834f4d46dff555b2e60c83cd526827b31 \
  --out /private/tmp/picogent-evidence-28cc214/runtime-boundary-matrix.json
```

The matrix recorded `HEAD=PASS`, `tree=CLEAN`, `PASS:8`,
`INCONCLUSIVE:1`, and `UNVERIFIED:2`.

Explicitly unchanged boundaries:

- `rendered-cross-platform=UNVERIFIED`
- `hostile-filesystem-toctou=UNVERIFIED`
- `release-authorization=INCONCLUSIVE`

This artifact is bound to the pre-documentation behavior SHA above and must not
be silently rebound to a later merge SHA.
