# v4 live-provider evidence at tip 251171f

Status: exact-head matrix `PASS` for live-provider connectivity and the bounded
`fixed-no-tool-v1` quality campaign at source
`251171f2972cc3a28b03cf35c222e77134cb65b4`. This checkpoint belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

This is a bounded self-reported observation. It is not independent provider or
account attestation and does not prove streaming, tool use, recovery,
cross-platform rendering, hostile-runtime safety, or release authorization.

## Provenance

```text
repository:  github.com/saiaathish/picogent
source:      251171f2972cc3a28b03cf35c222e77134cb65b4
source_ref:  origin/main (#527 merge; last non-docs tip before docs-only descendants)
provider:    codex
model:       gpt-5.6-luna
campaign:    connectivity + fixed-no-tool-v1
runtime:     go1.26.6 darwin/arm64
environment: task-owned-disposable
observed:    2026-09-07T04:11:11Z
```

The source worktree was clean and matched that exact SHA. The binary was built
from that worktree and run with `PICOGENT_PROVIDER=codex`,
`PICOGENT_MODEL=gpt-5.6-luna`, and `PICOGENT_ROUTER=off`. Every observation used
a disposable `PICOGENT_HOME` and workspace plus a task-owned copy of Codex
credentials. The credential copy, raw responses, stderr, and provider session
state were removed after digest generation.

Earlier tip-bound observations at `21153f3…` (#531) were invalidated for
`--behavior-sha` retention when non-docs `#527` merged as `251171f…` during that
PR's CI window. These observations rebind live/local-rendered evidence to the
current behavior tip.

## Collection

The exact connectivity prompt and the three canonical quality prompts were run
once each through the tip-built binary. Only prompt and normalized-result
SHA-256 digests, latency, no-tool and no-mutation assertions, timestamps, and
verdicts were retained.

Artifacts outside the checkout:

```text
/private/tmp/picogent-evidence-251171f/live-provider-evidence.json
/private/tmp/picogent-evidence-251171f/live-provider-quality-evidence.json
/private/tmp/picogent-evidence-251171f/runtime-boundary-matrix.json
```

Artifact SHA-256:

```text
connectivity ba3f8ba22039ee99c571765f3ee746a22fe60540e95a7075f97387e5c1015534
quality      fac533fd1c68a857513d52162d07467849192df86089881186744cb0c933f3ef
matrix       ad7aadafcb18483dc412a8b93744e2a0ea967f862192de1daf00d030cd0cc33c
```

## Observations

| Observation | Result SHA-256 | Latency | Verdict |
| --- | --- | ---: | --- |
| connectivity | `c3e38e6256a68d4c57f7644d794f45532d71799556c494c57ec567973e8ea8c7` | 3938 ms | `PASS` |
| `exact-token` | `46a0a304898ade94da30c281d5f6fa5d61ce783cfcea6f4735b19671bea364b0` | 3187 ms | `PASS` |
| `bounded-summary` | `09bd593a251629c185ac83ee6be32ffd5c91120433b5d64ccdf1e2c0997a2994` | 3591 ms | `PASS` |
| `constraint-following` | `c07d57c5cdfac32af7d21cb4a96ab1d394a125240387ff916b93d91f51c3ea65` | 2091 ms | `PASS` |

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

`rendered-cross-platform` stays exact-candidate bound to `18dc4a1…` aggregate
`e519f22a…` and is not reattached at this tip. After this evidence
documentation lands, the unchanged artifacts may be supplied from a clean
docs-only descendant with:

```sh
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha "$(git rev-parse HEAD)" \
  --behavior-sha 251171f2972cc3a28b03cf35c222e77134cb65b4
```

Acceptance still requires Git to prove every intervening touch is under
`docs/`; it does not rebind cross-platform rendering, hostile TOCTOU, or release
authorization.
