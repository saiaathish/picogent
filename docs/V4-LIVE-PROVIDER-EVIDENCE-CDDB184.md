# v4 live-provider evidence at tip cddb184

Status: exact-head matrix `PASS` for live-provider connectivity and the bounded
`fixed-no-tool-v1` quality campaign at source
`cddb184cc90de13423749aeb021440f016194a33`. Belongs to
[#453](https://github.com/saiaathish/picogent/issues/453).

Bounded self-reported observation only. Does not prove streaming, tool use,
hostile TOCTOU, or release authorization.

## Provenance

```text
source:      cddb184cc90de13423749aeb021440f016194a33
provider:    codex / gpt-5.6-luna
environment: task-owned-disposable
observed:    2026-09-07T05:34:37Z
```

Post-merge tip after [#541](https://github.com/saiaathish/picogent/pull/541)
(Windows collector wait-stage diagnostics). Non-docs tip races `#542`/`#544`
followed; retain this SHA as the stable behavior tip for the three-platform
rendered aggregate below.

## Digests

```text
connectivity c30bb0945daa9a0b6b9ad547a5f7e9003e1a8d2fb3c0d4d2504731fd340c8cf7
quality      00d7869933d27afa11cb845b41f8bb0acf9cca303271a279c9e3dc7b0e7bdae8
matrix       ebb29d8ec4a6003d5b5145a968020e3028c308a83d3f728b9fb72135eb1026e8
```

Artifacts: `/private/tmp/picogent-evidence-cddb184/`.

## Observations

| Observation | Result SHA-256 | Latency | Verdict |
| --- | --- | ---: | --- |
| connectivity | `05dc5c5d0202258c9b25d2c0dc47b4a232d4cff108b52629ee52da37990e41c7` | 2796 ms | `PASS` |
| `exact-token` | `4472a69b5af2f7cddada501ee917a82cf088b6403ff2d61f48f3379fdd55a93a` | 2808 ms | `PASS` |
| `bounded-summary` | `c7d3bc4f541a2441a3a0a130da4486f75bf310f8883091538cfd72e825d10100` | 3004 ms | `PASS` |
| `constraint-following` | `7ef6fa5dcf4e54b7f47cfb130b5d573ad9c375ca2c89877381f235681de75c06` | 2163 ms | `PASS` |

## Matrix

```text
PASS: 9  INCONCLUSIVE: 1  UNVERIFIED: 1
behavior_provenance=EXACT_HEAD
rendered-cross-platform=PASS
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
```

Retain across docs-only descendants with
`--behavior-sha cddb184cc90de13423749aeb021440f016194a33` for live/local
rendered rows only. Cross-platform remains exact-candidate bound to this SHA.
