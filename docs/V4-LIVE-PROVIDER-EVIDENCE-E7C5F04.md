# v4 live-provider evidence at tip e7c5f04

Status: exact-head matrix `PASS` for both live-provider connectivity and the
bounded fixed no-tool quality campaign at source
`e7c5f04d227c12eede0456d11a5ddcbb6636cee4`. This record belongs to
[#497](https://github.com/saiaathish/picogent/issues/497) under parent
[#453](https://github.com/saiaathish/picogent/issues/453).

This is a bounded self-reported observation. It is not an independent
attestation of provider or account identity, and it is not a streaming,
tool-use, recovery, cross-platform, hostile-runtime, or release-authorization
claim.

## Provenance

```text
repository:  github.com/saiaathish/picogent
source:      e7c5f04d227c12eede0456d11a5ddcbb6636cee4
source_ref:  origin/main (verified exact match before observation)
provider:    codex
campaign:    connectivity + fixed-no-tool-v1
runtime:     go1.26.6 darwin/arm64
environment: task-owned-disposable
observed:    2026-09-06T11:59:28Z (connectivity)
             2026-09-06T11:59:56Z (quality)
```

The source checkout was clean and `HEAD` matched the recorded full commit ID.
The Picogent binary was built from that checkout and run with
`PICOGENT_PROVIDER=codex`, `PICOGENT_MODEL=gpt-5.6-luna`, and
`PICOGENT_ROUTER=off` using disposable `PICOGENT_HOME` / workspace paths and a
copied task-owned Codex home. The temporary credential copy was removed after
artifact generation. Cleanup is operator-reported.

Author-host-local digest-only artifacts (not PR paths):

```text
/private/tmp/picogent-evidence-e7c5f04/live-provider-evidence.json
/private/tmp/picogent-evidence-e7c5f04/live-provider-quality-evidence.json
```

Artifact SHA-256:

```text
connectivity 162d66920e000b80a1eb60e670267a5ac58fbfb51f5eca335c1354d07bd84cbc
quality      9d69dd7675fb4cfef36b17396e0e993685818461e58841a276fd6cf2f99a2501
```

Raw provider responses, stderr captures, credentials, and provider session data
were not retained.

## Connectivity observation

| Field | Value |
| --- | --- |
| Prompt digest | `0c4e68894f41a3ce3194b17b931f22128600a4511ab42cfd69f22c17411c5887` |
| Result digest | `c3e38e6256a68d4c57f7644d794f45532d71799556c494c57ec567973e8ea8c7` |
| Tools | none observed |
| Workspace mutation | none observed |
| Verdict | `PASS` |

## Quality campaign (`fixed-no-tool-v1`)

| Case | Result digest | Latency | Tools | Mutation | Verdict |
| --- | --- | ---: | --- | --- | --- |
| `exact-token` | `46a0a304898ade94da30c281d5f6fa5d61ce783cfcea6f4735b19671bea364b0` | 2221 ms | none | none | `PASS` |
| `bounded-summary` | `ce069e1a98ae60a789828614b431c1f2a6eec6d8c3cdc9a21e35e07b1b00cf85` | 3084 ms | none | none | `PASS` |
| `constraint-following` | `c07d57c5cdfac32af7d21cb4a96ab1d394a125240387ff916b93d91f51c3ea65` | 2092 ms | none | none | `PASS` |

Latency budget was 5000 ms; observed max was 3084 ms.

## Exact-head matrix projection

With both live artifacts supplied against the same clean source SHA, the matrix
projected:

| Claim | Verdict |
| --- | --- |
| `live-provider-connectivity` | `PASS` |
| `live-provider-quality` | `PASS` |
| `rendered-cross-platform` | `UNVERIFIED` |
| `hostile-filesystem-toctou` | `UNVERIFIED` |
| `release-authorization` | `INCONCLUSIVE` |

## Explicit limits

- Does not prove provider identity beyond artifact self-report.
- Does not cover streaming, tool use, auth refresh, or sustained sessions.
- Does not upgrade rendered, hostile-TOCTOU, or release-authorization rows.
- Bound to pre-documentation behavior SHA `e7c5f04d227c12eede0456d11a5ddcbb6636cee4`
  and must not be silently rebound to a later merge SHA.
