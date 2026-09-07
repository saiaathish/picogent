# v4 live-provider evidence at tip d129899

Status: exact-head matrix `PASS` for live-provider connectivity and the bounded
`fixed-no-tool-v1` quality campaign at source
`d1298998cc000c40ad2fdaf71a24e17649e09e19`. This checkpoint belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

This is a bounded self-reported observation. It is not independent provider or
account attestation and does not prove streaming, tool use, recovery,
cross-platform rendering, hostile-filesystem TOCTOU safety, or release
authorization.

## Provenance

```text
repository:  github.com/saiaathish/picogent
source:      d1298998cc000c40ad2fdaf71a24e17649e09e19
source_ref:  origin/main (fetched and matched before observation)
provider:    codex
model:       gpt-5.6-luna
campaign:    connectivity + fixed-no-tool-v1
runtime:     go1.26.6 darwin/arm64
environment: task-owned-disposable
observed:    2026-09-07T01:13:54Z (connectivity)
             2026-09-07T01:14:01Z (quality)
```

The source worktree was clean and matched `origin/main`. The binary was built
from that worktree and run with `PICOGENT_PROVIDER=codex`,
`PICOGENT_MODEL=gpt-5.6-luna`, and `PICOGENT_ROUTER=off`. Each observation used
a disposable `PICOGENT_HOME` and workspace plus a task-owned copy of Codex
credentials. The copy, raw responses, stderr, and provider session state were
removed after digest generation.

The first complete attempt returned all four contract-conforming responses, but
the `exact-token` case took 6634 ms and exceeded the fixed 5000 ms budget. No
artifact was emitted from that attempt. One complete rerun produced the bounded
passing observation recorded below.

## Collection command pattern

```sh
go build -trimpath \
  -o /private/tmp/picogent-evidence-d129899/picogent \
  ./cmd/picogent

PICOGENT_HOME="$disposable_home" \
PICOGENT_CODEX_HOME="$disposable_codex_home" \
CODEX_HOME="$disposable_codex_home" \
PICOGENT_PROVIDER=codex \
PICOGENT_MODEL=gpt-5.6-luna \
PICOGENT_ROUTER=off \
/private/tmp/picogent-evidence-d129899/picogent run \
  --dir "$disposable_workspace" --yes "$fixed_prompt"
```

The second command was run once per canonical connectivity / quality prompt.
Only prompt and normalized-result SHA-256 digests, latency, no-tool and
no-mutation assertions, timestamps, and verdicts were retained.

Host-local artifacts outside the checkout:

```text
/private/tmp/picogent-evidence-d129899/live-provider-evidence.json
/private/tmp/picogent-evidence-d129899/live-provider-quality-evidence.json
/private/tmp/picogent-evidence-d129899/runtime-boundary-matrix.json
```

Artifact SHA-256:

```text
connectivity 38e33f9ebdb0f1af82bb6438a4a4fc739e959134d41531a01147a819ff9ef694
quality      a324dece5a08037774b8625ae98dbf9d4c36fefd92576a999e97ecccc9b0c456
matrix       d6d8a1851d628812445d80cf1edc425721aab9395fb29f22c50d836ad7f62ac3
```

## Observations

| Observation | Result SHA-256 | Latency | Verdict |
| --- | --- | ---: | --- |
| connectivity | `c3e38e6256a68d4c57f7644d794f45532d71799556c494c57ec567973e8ea8c7` | 2335 ms | `PASS` |
| `exact-token` | `46a0a304898ade94da30c281d5f6fa5d61ce783cfcea6f4735b19671bea364b0` | 2129 ms | `PASS` |
| `bounded-summary` | `26fc56be83e3511c43bd534ed746ffc0577436c769810b6207c3285b9ad79a36` | 3115 ms | `PASS` |
| `constraint-following` | `c07d57c5cdfac32af7d21cb4a96ab1d394a125240387ff916b93d91f51c3ea65` | 2186 ms | `PASS` |

All accepted quality cases stayed below the declared 5000 ms budget and
recorded `tools_used=false` and `mutation_observed=false`.

## Exact-head matrix validation

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/private/tmp/picogent-evidence-d129899/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/private/tmp/picogent-evidence-d129899/live-provider-quality-evidence.json \
PICOGENT_RENDERED_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_PLATFORM_ARTIFACT=/private/tmp/picogent-evidence-d129899/rendered-platform-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha d1298998cc000c40ad2fdaf71a24e17649e09e19 \
  --out /private/tmp/picogent-evidence-d129899/runtime-boundary-matrix.json
```

The matrix recorded `HEAD=PASS`, `tree=CLEAN`,
`behavior_provenance=EXACT_HEAD`, `PASS:8`, `INCONCLUSIVE:1`, and
`UNVERIFIED:2`.

Explicitly unchanged boundaries:

- `rendered-cross-platform=UNVERIFIED`
- `hostile-filesystem-toctou=UNVERIFIED`
- `release-authorization=INCONCLUSIVE`

## Docs-only tip retention

After evidence-only documentation lands, rerun from the clean new tip while
leaving the external artifacts bound to this observation SHA:

```sh
candidate_sha="$(git rev-parse HEAD)"
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/private/tmp/picogent-evidence-d129899/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/private/tmp/picogent-evidence-d129899/live-provider-quality-evidence.json \
PICOGENT_RENDERED_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_PLATFORM_ARTIFACT=/private/tmp/picogent-evidence-d129899/rendered-platform-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha "$candidate_sha" \
  --behavior-sha d1298998cc000c40ad2fdaf71a24e17649e09e19 \
  --out /private/tmp/picogent-evidence-d129899/runtime-boundary-matrix-retained.json
```

Acceptance requires Git to prove that the candidate is a descendant and every
intervening touched path is under `docs/`. The resulting report must retain
`PASS:8`, `INCONCLUSIVE:1`, `UNVERIFIED:2` and record
`behavior_provenance=DOCS_ONLY_DESCENDANT`. Any intervening non-docs change
rejects these artifacts.
