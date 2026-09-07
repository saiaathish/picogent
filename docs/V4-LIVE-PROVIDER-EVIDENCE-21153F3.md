# v4 live-provider evidence at tip 21153f3

Status: exact-head matrix `PASS` for live-provider connectivity and the bounded
`fixed-no-tool-v1` quality campaign at source
`21153f3a5626e727a5b29f9cacb351fcf4ae4e7f`. This checkpoint belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

This is a bounded self-reported observation. It is not independent provider or
account attestation and does not prove streaming, tool use, recovery,
cross-platform rendering, hostile-runtime safety, or release authorization.

## Provenance

```text
repository:  github.com/saiaathish/picogent
source:      21153f3a5626e727a5b29f9cacb351fcf4ae4e7f
source_ref:  origin/main (fetched and matched before observation)
provider:    codex
model:       gpt-5.6-luna
campaign:    connectivity + fixed-no-tool-v1
runtime:     go1.26.6 darwin/arm64
environment: task-owned-disposable
observed:    2026-09-07T03:52:47Z
```

The source worktree was clean and matched `origin/main`. The binary was built
from that worktree and run with `PICOGENT_PROVIDER=codex`,
`PICOGENT_MODEL=gpt-5.6-luna`, and `PICOGENT_ROUTER=off`. Every observation used
a disposable `PICOGENT_HOME` and workspace plus a task-owned copy of Codex
credentials. The credential copy, raw responses, stderr, and provider session
state were removed after digest generation.

Non-docs tip `#524` (and intervening non-docs merges through `#526`) invalidated
`--behavior-sha` retention from `18dc4a1…`, so these observations were collected
afresh at `21153f3…`.

## Collection

The exact connectivity prompt and the three canonical quality prompts were run
once each through the tip-built binary. Only prompt and normalized-result
SHA-256 digests, latency, no-tool and no-mutation assertions, timestamps, and
verdicts were retained.

Artifacts outside the checkout:

```text
/private/tmp/picogent-evidence-21153f3/live-provider-evidence.json
/private/tmp/picogent-evidence-21153f3/live-provider-quality-evidence.json
/private/tmp/picogent-evidence-21153f3/runtime-boundary-matrix.json
```

Artifact SHA-256:

```text
connectivity d68090f2aaab6504fd90a2faa01aaac8426b8a45268f207f05f8b6e6d20a32d6
quality      2f21bc1cee373204679fb5a15a7075a5f85957ac581e2bae2f6c489562f48c5c
matrix       ed7f9eae4b5a1f41bc476fadc3bb5e18085fd80b6257a9ba9f35d1318e68a332
```

## Observations

| Observation | Result SHA-256 | Latency | Verdict |
| --- | --- | ---: | --- |
| connectivity | `c3e38e6256a68d4c57f7644d794f45532d71799556c494c57ec567973e8ea8c7` | 4047 ms | `PASS` |
| `exact-token` | `46a0a304898ade94da30c281d5f6fa5d61ce783cfcea6f4735b19671bea364b0` | 2772 ms | `PASS` |
| `bounded-summary` | `385702ab9ee2eb99273df85a51a89f056296b112900226ad93ad37ab2aae44bc` | 2836 ms | `PASS` |
| `constraint-following` | `c07d57c5cdfac32af7d21cb4a96ab1d394a125240387ff916b93d91f51c3ea65` | 2459 ms | `PASS` |

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
  --behavior-sha 21153f3a5626e727a5b29f9cacb351fcf4ae4e7f
```

Acceptance still requires Git to prove every intervening touch is under
`docs/`; it does not rebind cross-platform rendering, hostile TOCTOU, or release
authorization.
