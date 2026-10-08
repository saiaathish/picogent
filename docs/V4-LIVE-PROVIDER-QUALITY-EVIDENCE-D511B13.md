# v4 live-provider quality evidence at exact main `d511b13`

Status: the fixed no-tool v2 campaign passed on the exact clean `main` source
for [#754](https://github.com/saiaathish/picogent/issues/754). This is a small
quality observation, not a v3-versus-v4 comparison or release authorization.

## Provenance

```text
repository:  github.com/saiaathish/picogent
source:      d511b13483ceb8d81ba439de772db3a19fa4ad0d
source_ref:  origin/main (verified exact match before and after the campaign)
provider:    codex (selected by Picogent's default configuration)
campaign:    fixed-no-tool-v2
runtime:     go1.26.6 darwin/arm64
environment: task-owned-disposable
observed:    2026-10-08T22:12:21Z
```

The source worktree was clean at the full recorded SHA. The Picogent binary was
built from that exact source. Each accepted observation ran in a separate
process and empty workspace with a per-case `PICOGENT_HOME` and a copied,
0600-mode Codex login under `PICOGENT_CODEX_HOME`. No paid API key or explicit
provider/model override was used; the existing Codex login was used with
Picogent's defaults. Provider/account identity is not independently attested.

The retained secret-free v2 artifact is outside the checkout at:

```text
/private/tmp/picogent-live-754.9KQWwb/evidence/live-provider-quality-evidence.json
```

Artifact SHA-256:

```text
79afe5416adf97127f86004b4eb3c1c5c8b30806ba58847f470dbda695753116
```

The artifact stores only canonical prompt/result digests, bounded latencies,
case verdicts, and explicit tool/mutation assertions. The temporary run
directories containing raw provider output, stderr, trace logs, and copied
credentials were moved to the user's Trash after validation; they are
recoverable until Trash is emptied and are not included in this PR or retained
evidence. The bounded-summary's wording remains self-reported because raw
output is intentionally not retained in the evidence artifact.

## Fixed campaign observations

Each case used the corresponding fixed prompt identity in the v2 runtime
boundary contract, ran as a fresh process, and exited successfully. Result
digests are over assistant response bytes (excluding the CLI's terminal line
feed, if present).

| Case | Result digest | Latency | Tool events / stderr markers | Workspace entries | Verdict |
| --- | --- | ---: | --- | ---: | --- |
| `exact-token` | `46a0a304898ade94da30c281d5f6fa5d61ce783cfcea6f4735b19671bea364b0` | 2400 ms | 0 / 0 | 0 | `PASS` |
| `bounded-summary` | `4fd8ecba2cda7e7c386fcf0d2e5f6e083a7c713cf60d04a14ed81587675ff3a3` | 1885 ms | 0 / 0 | 0 | `PASS` |
| `constraint-following` | `c07d57c5cdfac32af7d21cb4a96ab1d394a125240387ff916b93d91f51c3ea65` | 1806 ms | 0 / 0 | 0 | `PASS` |

All cases were below the 5000 ms budget. The exact-token and three-word
responses matched their canonical byte digests. The bounded-summary observer
check found one nonempty sentence with purpose-related language. For every
case, Picogent's persisted trace had zero `tool_start`/`tool_end` events, stderr
had zero tool-start markers, and the disposable workspace remained empty.
These are bounded operator observations, not a general tool-use guarantee.

## Exact-head matrix projection

The digest-only matrix report is outside the checkout at:

```text
/private/tmp/picogent-live-754.9KQWwb/evidence/runtime-boundary-matrix.json
```

Its SHA-256 is
`9077de1d036673e40ba548734d5bad5ca471114ba005986e822db2b92504012a`. The
runtime-boundary collector recorded `tree=CLEAN` and
`behavior_provenance=EXACT_HEAD`; the summary was `PASS:1`, `UNVERIFIED:11`.
A separate no-quality baseline matrix is retained at
`/private/tmp/picogent-live-754.9KQWwb/evidence/runtime-boundary-matrix-without-quality.json`
(SHA-256 `c10cf9bdc90e2519aa0532d297f0396bbeb7772087a03cb4f0556e0b6f389420`)
with `UNVERIFIED:12`. Comparison showed that the only changed verdict was
`live-provider-quality` from `UNVERIFIED` to `PASS`. All other 11 claim
verdicts remained unchanged. In particular, connectivity, rendered behavior,
hostile-runtime boundaries, and release-authorization remain unverified; this
observation does not authorize a release.

The evidence and matrix are bound to behavior SHA
`d511b13483ceb8d81ba439de772db3a19fa4ad0d`, before this documentation change.
Do not reinterpret them as exact-head evidence for this documentation branch
or its merge. Any projection through later commits must use the documented
docs-only continuity check.
