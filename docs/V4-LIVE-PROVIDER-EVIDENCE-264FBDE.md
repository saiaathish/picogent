# v4 live-provider evidence at tip 264fbde

Status: exact-head matrix `PASS` for live-provider connectivity and the bounded
`fixed-no-tool-v1` quality campaign at source
`264fbde2b45609e6e85e175efe216e8f63509ab8`. This checkpoint belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

This is a bounded self-reported observation. It is not independent provider or
account attestation and does not prove streaming, tool use, recovery,
cross-platform rendering, hostile-runtime safety, or release authorization.

## Provenance

```text
repository:  github.com/saiaathish/picogent
source:      264fbde2b45609e6e85e175efe216e8f63509ab8
source_ref:  origin/main (fetched and matched before observation)
provider:    codex
model:       gpt-5.6-luna
campaign:    connectivity + fixed-no-tool-v1
runtime:     go1.26.6 darwin/arm64
environment: task-owned-disposable
observed:    2026-09-07T00:05:05Z (connectivity)
             2026-09-07T00:05:11Z (quality)
```

The source worktree was clean and matched `origin/main`. The binary was built
from that worktree and run with `PICOGENT_PROVIDER=codex`,
`PICOGENT_MODEL=gpt-5.6-luna`, and `PICOGENT_ROUTER=off`. Each observation used
a disposable `PICOGENT_HOME` and workspace plus a task-owned copy of Codex
credentials. The copy, raw responses, stderr, and provider session state were
removed after digest generation.

## Collection command pattern

```sh
go build -trimpath \
  -o /private/tmp/picogent-evidence-264fbde/picogent \
  ./cmd/picogent

PICOGENT_HOME="$disposable_home" \
PICOGENT_CODEX_HOME="$disposable_codex_home" \
CODEX_HOME="$disposable_codex_home" \
PICOGENT_PROVIDER=codex \
PICOGENT_MODEL=gpt-5.6-luna \
PICOGENT_ROUTER=off \
/private/tmp/picogent-evidence-264fbde/picogent run \
  --dir "$disposable_workspace" --yes "$fixed_prompt"
```

The second command was run once per canonical connectivity / quality prompt.
Only prompt and normalized-result SHA-256 digests, latency, no-tool and
no-mutation assertions, timestamps, and verdicts were retained.

Host-local artifacts outside the checkout:

```text
/private/tmp/picogent-evidence-264fbde/live-provider-evidence.json
/private/tmp/picogent-evidence-264fbde/live-provider-quality-evidence.json
/private/tmp/picogent-evidence-264fbde/runtime-boundary-matrix.json
```

Artifact SHA-256:

```text
connectivity d11e2a1f46f38d07956af05a794381a89eb15c3482c6a62c3a1e7dec46d711bf
quality      12215bff62e17b8631333be50350cdee7fcbe1d4b7322353651856c49a06a41a
matrix       4bd71b1829ff0ad217159b60c6bd84c74a0de20c61e077670106a4b33c7b4fbf
```

## Observations

| Observation | Result SHA-256 | Latency | Verdict |
| --- | --- | ---: | --- |
| connectivity | `c3e38e6256a68d4c57f7644d794f45532d71799556c494c57ec567973e8ea8c7` | 3659 ms | `PASS` |
| `exact-token` | `46a0a304898ade94da30c281d5f6fa5d61ce783cfcea6f4735b19671bea364b0` | 2060 ms | `PASS` |
| `bounded-summary` | `990b5257e4a65fd9cf4710c89c32c0eaf0825ee547cf2b64e2702228eae5af04` | 2465 ms | `PASS` |
| `constraint-following` | `c07d57c5cdfac32af7d21cb4a96ab1d394a125240387ff916b93d91f51c3ea65` | 1770 ms | `PASS` |

All accepted quality cases stayed below the declared 5000 ms budget and
recorded `tools_used=false` and `mutation_observed=false`.

## Matrix validation

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/private/tmp/picogent-evidence-264fbde/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/private/tmp/picogent-evidence-264fbde/live-provider-quality-evidence.json \
PICOGENT_RENDERED_PLATFORM_EVIDENCE=1 \
PICOGENT_RENDERED_PLATFORM_ARTIFACT=/private/tmp/picogent-evidence-264fbde/rendered-platform-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha 264fbde2b45609e6e85e175efe216e8f63509ab8 \
  --out /private/tmp/picogent-evidence-264fbde/runtime-boundary-matrix.json
```

The matrix recorded `HEAD=PASS`, `tree=CLEAN`, `PASS:8`,
`INCONCLUSIVE:1`, and `UNVERIFIED:2`.

Explicitly unchanged boundaries:

- `rendered-cross-platform=UNVERIFIED`
- `hostile-filesystem-toctou=UNVERIFIED`
- `release-authorization=INCONCLUSIVE`

This artifact is bound to the pre-documentation source SHA above and must not
be silently rebound to a later merge SHA.

Under the behavior-SHA continuity contract, these artifacts may be supplied
with `--behavior-sha 264fbde...` only when the candidate is a docs-only
descendant. They are not eligible at the merge that introduces that contract,
because Go code and tests changed after `264fbde`. Re-observe once at the
contract merge SHA; subsequent evidence-documentation commits can reuse that
new behavior-bound observation while their entire intervening diff stays under
`docs/`.
