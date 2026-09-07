# v4 live-provider evidence at tip 18dc4a1

Status: exact-head matrix `PASS` for live-provider connectivity and the bounded
`fixed-no-tool-v1` quality campaign at source
`18dc4a1ca1137ab78dfd0102848eb99c417653cc`. This checkpoint belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

This is a bounded self-reported observation. It is not independent provider or
account attestation and does not prove streaming, tool use, recovery,
cross-platform rendering, hostile-runtime safety, or release authorization.

## Provenance

```text
repository:  github.com/saiaathish/picogent
source:      18dc4a1ca1137ab78dfd0102848eb99c417653cc
source_ref:  origin/main (fetched and matched before observation)
provider:    codex
model:       gpt-5.6-luna
campaign:    connectivity + fixed-no-tool-v1
runtime:     go1.26.6 darwin/arm64
environment: task-owned-disposable
observed:    2026-09-07T02:02:00Z (connectivity)
             2026-09-07T02:02:10Z (quality)
```

The source worktree was clean and matched `origin/main`. The binary was built
from that worktree and run with `PICOGENT_PROVIDER=codex`,
`PICOGENT_MODEL=gpt-5.6-luna`, and `PICOGENT_ROUTER=off`. Every observation used
a disposable `PICOGENT_HOME` and workspace plus a task-owned copy of Codex
credentials. The credential copy, raw responses, stderr, and provider session
state were removed after digest generation.

## Collection

The exact connectivity prompt and the three canonical quality prompts were run
once each through the tip-built binary. Only prompt and normalized-result
SHA-256 digests, latency, no-tool and no-mutation assertions, timestamps, and
verdicts were retained.

Artifacts outside the checkout:

```text
/private/tmp/picogent-evidence-post519/live-provider-evidence.json
/private/tmp/picogent-evidence-post519/live-provider-quality-evidence.json
/private/tmp/picogent-evidence-post519/runtime-boundary-matrix.json
```

Artifact SHA-256:

```text
connectivity 9f1847405da0aa7ba27fc65f5e6599dd77e967de4d8fa9b8590d015aed167fbd
quality      4b962d754ea1b7093b71d69e6534820576f6de38120649dc279d2007d2dc322f
matrix       2dfc730aab19b60351d2e3e26f7497f434e5dea454974db13cba457b11aa7068
```

## Observations

| Observation | Result SHA-256 | Latency | Verdict |
| --- | --- | ---: | --- |
| connectivity | `c3e38e6256a68d4c57f7644d794f45532d71799556c494c57ec567973e8ea8c7` | 5411 ms | `PASS` |
| `exact-token` | `46a0a304898ade94da30c281d5f6fa5d61ce783cfcea6f4735b19671bea364b0` | 2864 ms | `PASS` |
| `bounded-summary` | `b65dd9de597552288917bc47df9a99093d5c238c09d93393543e2e0ecf219725` | 3943 ms | `PASS` |
| `constraint-following` | `c07d57c5cdfac32af7d21cb4a96ab1d394a125240387ff916b93d91f51c3ea65` | 2622 ms | `PASS` |

All accepted quality cases stayed below the declared 5000 ms budget and
recorded `tools_used=false` and `mutation_observed=false`. Connectivity has no
quality-campaign latency budget.

## Exact-head matrix

With the matching connectivity, quality, and Darwin rendered artifacts
supplied, the matrix recorded:

```text
HEAD=PASS  tree=CLEAN  behavior_provenance=EXACT_HEAD
PASS: 8  INCONCLUSIVE: 1  UNVERIFIED: 2
rendered-cross-platform=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

The non-docs #519 merge invalidated the earlier `d129899` behavior retention,
so these observations were collected afresh at `18dc4a1`. After this evidence
documentation lands, the unchanged artifacts may be supplied from a clean
docs-only descendant with:

```sh
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha "$(git rev-parse HEAD)" \
  --behavior-sha 18dc4a1ca1137ab78dfd0102848eb99c417653cc
```

Acceptance still requires Git to prove every intervening touch is under
`docs/`; it does not rebind cross-platform rendering, hostile TOCTOU, or release
authorization.
