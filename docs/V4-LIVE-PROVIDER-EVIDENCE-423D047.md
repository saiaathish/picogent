# v4 live-provider evidence at tip 423d047

Status: exact-head matrix `PASS` for live-provider connectivity and the bounded
`fixed-no-tool-v1` quality campaign at source
`423d0471c1864651473c2f1828b67066885f79bd`. Belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

Bounded self-reported observation only. Does not prove streaming, tool use,
cross-platform rendering, hostile TOCTOU, or release authorization.

## Provenance

```text
source:      423d0471c1864651473c2f1828b67066885f79bd
provider:    codex / gpt-5.6-luna
environment: task-owned-disposable
observed:    2026-09-07T05:50:23Z
```

Non-docs `#542`/`#544` (plus docs `#545`) invalidated prior tip evidence at
behavior `37d9206…` / `cddb184…`, so observations were recollected at this tip.

## Digests

```text
connectivity 5752fa05b42ae4019885a9a7baeb123a653746a6b0548e214f9be19c2ab18e15
quality      decbec1d7990fb4e40c0c577b0e968f3d02e4859e040cd29bd1b78fdcd5cfe60
matrix       062aa3f0d476498198b4eae3aea682ae76a0a2d14d870f38488d6e03f33710f2
```

Artifacts: `/private/tmp/picogent-evidence-423d047/`.

## Observations

| Observation | Result SHA-256 | Latency | Verdict |
| --- | --- | ---: | --- |
| connectivity | `05dc5c5d0202258c9b25d2c0dc47b4a232d4cff108b52629ee52da37990e41c7` | 4476 ms | `PASS` |
| `exact-token` | `4472a69b5af2f7cddada501ee917a82cf088b6403ff2d61f48f3379fdd55a93a` | 2435 ms | `PASS` |
| `bounded-summary` | `70f4c100579b3cf13293a7c9e99049e56396b31769bcec11da55cccc4fe92d7e` | 2985 ms | `PASS` |
| `constraint-following` | `7ef6fa5dcf4e54b7f47cfb130b5d573ad9c375ca2c89877381f235681de75c06` | 3069 ms | `PASS` |

## Matrix

Live+local matrix digest above was recorded before tip Windows packaging. That
earlier projection kept `rendered-cross-platform=UNVERIFIED`:

```text
PASS: 9  INCONCLUSIVE: 1  UNVERIFIED: 2
behavior_provenance=EXACT_HEAD
hostile-parent-swap-confinement=PASS
rendered-cross-platform=UNVERIFIED
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

After tip Windows recollection and aggregate packaging
([V4-RENDERED-WINDOWS-EVIDENCE-423D047.md](V4-RENDERED-WINDOWS-EVIDENCE-423D047.md),
[V4-RENDERED-CROSS-PLATFORM.md](V4-RENDERED-CROSS-PLATFORM.md)), exact-SHA
matrix with the tip aggregate plus live/local artifacts projects:

```text
PASS: 10  INCONCLUSIVE: 1  UNVERIFIED: 1
behavior_provenance=EXACT_HEAD
rendered-cross-platform=PASS
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

Retain across docs-only descendants with
`--behavior-sha 423d0471c1864651473c2f1828b67066885f79bd` for live/local
rendered rows and the tip cross-platform aggregate.
