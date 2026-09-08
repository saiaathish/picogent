# v4 live-provider evidence at tip `1ce2059`

Status: exact-head matrix `PASS` for live-provider connectivity and the bounded
`fixed-no-tool-v1` quality campaign at source
`1ce2059fc352b5d32b5da3adbaeb1ecb48834db7`. This checkpoint belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

This is a bounded self-reported observation. It is not independent provider or
account attestation and does not prove streaming, tool use, authentication
refresh, recovery, cross-platform rendering, hostile-runtime safety, or release
authorization.

## Provenance

```text
repository:  github.com/saiaathish/picogent
source:      1ce2059fc352b5d32b5da3adbaeb1ecb48834db7
source_ref:  origin/main (verified exact match before observation)
provider:    codex
model:       gpt-5.6-luna
campaign:    connectivity + fixed-no-tool-v1
runtime:     go1.26.6 darwin/arm64
environment: task-owned-disposable
observed:    2026-09-08T11:41:16Z (digest artifact timestamp)
```

The source checkout used for the live run was clean and matched the exact
candidate SHA. Picogent ran with `PICOGENT_PROVIDER=codex`,
`PICOGENT_MODEL=gpt-5.6-luna`, and `PICOGENT_ROUTER=off`. The provider
credential was copied into a disposable task-owned Codex home; Picogent state
and each case workspace were disposable. Raw provider output, stderr, and the
temporary credential are not release evidence and are not retained here.

## Collection

The connectivity prompt and each fixed quality prompt ran once in a fresh
Picogent process. The retained artifacts contain only canonical prompt/result
digests, bounded timings, and explicit no-tool/no-mutation assertions.

| Observation | Result SHA-256 | Latency | Workspace entries | Verdict |
| --- | --- | ---: | ---: | --- |
| connectivity | `c3e38e6256a68d4c57f7644d794f45532d71799556c494c57ec567973e8ea8c7` | 4743 ms | 0 | `PASS` |
| `exact-token` | `46a0a304898ade94da30c281d5f6fa5d61ce783cfcea6f4735b19671bea364b0` | 2100 ms | 0 | `PASS` |
| `bounded-summary` | `0c00dd93975cde26f0e82a1d27b331b4b0370df6c9b15ab1da840c3a29c463b2` | 2551 ms | 0 | `PASS` |
| `constraint-following` | `c07d57c5cdfac32af7d21cb4a96ab1d394a125240387ff916b93d91f51c3ea65` | 3021 ms | 0 | `PASS` |

The quality campaign stayed below its 5000 ms per-case budget. The exact-token
and three-word cases matched their canonical response contracts; the summary
case returned one non-empty line. `PICOGENT_ROUTER=off`, zero workspace
entries, and zero captured stderr bytes supported the explicit
`tools_used=false` and `mutation_observed=false` assertions. These remain
operator-observed assertions, not independent provider attestation.

## Retained artifacts

Artifacts were retained outside the checkout under the task-owned path
`/private/tmp/picogent-evidence-1ce2059/`:

```text
live-provider-evidence.json
  sha256 c2301ee6bbd922e3b8bb79bbff4ae47dea4aac46e2df06fc7e5f1315ccb111b6
live-provider-quality-evidence.json
  sha256 8f8d853f60df019591fbcb293c2f1131eb379b31e4e1e1c12e41802c0a3aba3f
runtime-boundary-matrix-full.json
  sha256 c0e56a26407c19828a9e1a76e6d8a8e229cdaf6ffc1c7b0b7056ba52ea7ed6c7
```

## Exact-head matrix

With the matching live-provider and rendered evidence artifacts supplied, the
clean exact-head matrix recorded:

```text
candidate_sha=1ce2059fc352b5d32b5da3adbaeb1ecb48834db7
head_match=PASS  tree=CLEAN  behavior_provenance=EXACT_HEAD
summary=PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1
live-provider-connectivity=PASS
live-provider-quality=PASS
rendered-platform-local=PASS
rendered-cross-platform=PASS
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

The rendered rows are independently documented in
[V4-RENDERED-CROSS-PLATFORM-EVIDENCE-1CE2059.md](V4-RENDERED-CROSS-PLATFORM-EVIDENCE-1CE2059.md).
This live-provider record does not upgrade any rendered, hostile-runtime, or
release-authorization claim by itself.

## Reproduction boundary

Reproduce from a clean checkout at the exact source SHA, using a disposable
provider home and workspace. The matrix must be run while all referenced
artifacts remain outside the checkout:

```sh
PICOGENT_LIVE_PROVIDER_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_ARTIFACT=/private/tmp/picogent-evidence-1ce2059/live-provider-evidence.json \
PICOGENT_LIVE_PROVIDER_QUALITY_EVIDENCE=1 \
PICOGENT_LIVE_PROVIDER_QUALITY_ARTIFACT=/private/tmp/picogent-evidence-1ce2059/live-provider-quality-evidence.json \
go run ./cmd/runtime-boundary-matrix \
  --workspace . \
  --candidate-sha 1ce2059fc352b5d32b5da3adbaeb1ecb48834db7
```

The artifact and matrix are bound to this exact behavior SHA. A later
non-documentation change requires a fresh observation. A later docs-only
descendant may use the repository's documented `--behavior-sha` continuity
contract for the live rows, but that exception does not authorize a release or
rebind the cross-platform aggregate.
