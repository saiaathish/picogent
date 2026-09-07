# v4 live-provider evidence at tip 37d9206

Status: exact-head matrix `PASS` for live-provider connectivity and the bounded
`fixed-no-tool-v1` quality campaign at source
`37d9206f4512be61fe1b751359c176af85ff6646`. Belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

Bounded self-reported observation only. Does not prove streaming, tool use,
cross-platform rendering, hostile TOCTOU, or release authorization.

## Provenance

```text
source:      37d9206f4512be61fe1b751359c176af85ff6646
provider:    codex / gpt-5.6-luna
environment: task-owned-disposable
observed:    2026-09-07T04:45:45Z
```

Non-docs `#536`/`#538` invalidated prior `15201f5…` retention mid-flight for
[#539](https://github.com/saiaathish/picogent/pull/539), so observations were
recollected at this tip.

## Digests

```text
connectivity f0ca76c18fc7e4f173982c3ea569407422bc677691c31c225d7f277c4639e9ac
quality      6a8195928438d2def8e92a3b63b831826222a0d3bac30f9b486bf11f9b0bc0c7
matrix       87bf8b5091cb964948b8c2e71f469133c996878f1be224bcbdb5fc0bde770b08
```

Artifacts: `/private/tmp/picogent-evidence-37d9206/`.

## Observations

| Observation | Result SHA-256 | Latency | Verdict |
| --- | --- | ---: | --- |
| connectivity | `c3e38e6256a68d4c57f7644d794f45532d71799556c494c57ec567973e8ea8c7` | 3237 ms | `PASS` |
| `exact-token` | `46a0a304898ade94da30c281d5f6fa5d61ce783cfcea6f4735b19671bea364b0` | 1964 ms | `PASS` |
| `bounded-summary` | `42edaef46c29991f9a9b465f8468ab6f9b53280923ae9e424c4e7fb93645717f` | 2733 ms | `PASS` |
| `constraint-following` | `c07d57c5cdfac32af7d21cb4a96ab1d394a125240387ff916b93d91f51c3ea65` | 2937 ms | `PASS` |

## Matrix

```text
PASS: 8  INCONCLUSIVE: 1  UNVERIFIED: 2
behavior_provenance=EXACT_HEAD
rendered-cross-platform=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

Retain across docs-only descendants with
`--behavior-sha 37d9206f4512be61fe1b751359c176af85ff6646`.
