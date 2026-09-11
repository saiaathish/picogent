# v4 rendered cross-platform evidence at exact candidate `eabf8d6`

Status: `PASS` for the rendered cross-platform recovery claim only. This
record belongs to [#453](https://github.com/saiaathishkarthik/picogent/issues/453).
It does not authorize a release, upgrade the broad hostile-filesystem residual,
or claim v4 completion.

## Candidate and common flow

All three observations used the exact clean candidate
`eabf8d6e35322170f7ab19dbd30d3cfb576f422c` and the same task-owned
`rendered-recovery` fixture:

1. load a fresh fixture in Safe mode;
2. submit the contained recovery task;
3. observe the rendered permission controls and click `Allow`;
4. observe the one-file change and click `Undo last change`;
5. stop the fixture, start a fresh process, and verify durable history without
   a stale Undo control.

Each platform record uses schema
`picogent.v4.rendered-platform-evidence.v1`, environment
`task-owned-disposable`, and verdict `PASS`. The retained records contain
bounded identity and SHA-256 values; no DOM, URL, credential, transcript, or
screenshot bytes are committed here.

## Platform records

| Platform | Collection | Browser | Observation SHA-256 | Screenshot-set SHA-256 | Platform-record SHA-256 |
| --- | --- | --- | --- | --- | --- |
| `darwin/arm64` | local task-owned collection | `playwright-chromium-headless-151.0.7922.34` | `79cdf6056ea8dd73deb7742cf5c4f3daa4b8a63eeac69cf23bf1be112ce4913c` | `f6e1b66ff96bb39019d154c14f2cf7e3960f5c90425500f3fea18ebe758650d5` | `c09ab552f968b6e33ca271a095c8f9bbf8b237c341e677f027b0edf076550c08` |
| `linux/amd64` | [hosted run 34589279428](https://github.com/saiaathish/picogent/actions/runs/34589279428) | `playwright-chromium-headless-134.0.6998.35` | `101b8daf93b8fe4f630da16d65bca669e6a17aa57ba42e610b0a6a352120623f` | `0ee2a63c7004397ff3c9b1928d0639c82dcc8048144804a2465d8f1d5a59b07e` | `b378a4fa2c25c285f87ffa4e98fe05e83f13fd06ac6dc54c1cf1a113164cb881` |
| `windows/amd64` | [hosted run 34589302297](https://github.com/saiaathishkarthik/picogent/actions/runs/34589302297) | `chrome-headless-152.0.7977.83` | `c5e17ea8614267070288bf7826ba7de54bd3438053fc849b67afbd472e1e0641` | `cfb8f938f2715ac61429ed0f63a85917f7587e610edd87953b5f4c9ee095a802` | `7c756a1bf5a0a1d242e140ff1ec7d3f1625fbef36f6864bf9a5ceaf05a2fe341` |

All three platform records report `source_tree_modified=false` and the common
candidate SHA. The Darwin collector passed all 18 required checks. The hosted
Linux and Windows collectors completed the same allow → undo → fresh-process
reload flow, verified their fixture exits, and uploaded digest-only records
plus screenshots.

## Aggregate and exact-candidate matrix

The aggregate validator accepted exactly one valid record for each required
platform:

```text
schema=picogent.v4.rendered-cross-platform-evidence.v1
candidate_sha=eabf8d6e35322170f7ab19dbd30d3cfb576f422c
required_platforms=darwin,linux,windows
verdict=PASS
source_tree_modified=false
aggregate_sha256=e59d54ebd6f75403974744432b03e28ba2725654c4a11e264a868d4842a0f681
```

The aggregate was retained outside the checkout at
`/private/tmp/picogent-rendered-eabf8d6-cross-platform-evidence.json`. Its
platform rows are Darwin/arm64, Linux/amd64, and Windows/amd64, each with
`verdict=PASS` and the common candidate SHA.

The exact-head runtime matrix was generated from a clean checkout at the same
candidate with both the Darwin local artifact and the cross-platform aggregate
supplied:

```text
candidate_sha=eabf8d6e35322170f7ab19dbd30d3cfb576f422c
behavior_sha=eabf8d6e35322170f7ab19dbd30d3cfb576f422c
behavior_provenance=EXACT_HEAD
head_match=PASS
tree=CLEAN
rendered-platform-local=PASS
rendered-cross-platform=PASS
hostile-filesystem-toctou=UNVERIFIED
live-provider-connectivity=UNVERIFIED
live-provider-quality=UNVERIFIED
release-authorization=INCONCLUSIVE
summary=PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3
runtime_matrix_sha256=32c49185692511abf753e678d682b668bda025bf14741b5877e3342e53c30e17
```

The platform-specific matrices separately reported
`rendered-platform-local=PASS` for Darwin/arm64, Linux/amd64, and
Windows/amd64. The exact-head matrix did not receive live-provider artifacts;
those rows therefore remain fail-closed `UNVERIFIED` and are not downgraded or
replaced by this rendered packet.

## Operator retention

The platform records, screenshots, aggregate, and matrix remain outside the
checkout at:

```text
/private/tmp/picogent-rendered-eabf8d6-darwin-evidence/
/private/tmp/picogent-rendered-eabf8d6-linux/rendered-linux-owned-browser-eabf8d6e35322170f7ab19dbd30d3cfb576f422c/picogent-linux-rendered-evidence/
/private/tmp/picogent-rendered-eabf8d6-windows/rendered-windows-evidence-eabf8d6e35322170f7ab19dbd30d3cfb576f422c/
/private/tmp/picogent-rendered-eabf8d6-cross-platform-evidence.json
/private/tmp/picogent-rendered-eabf8d6-runtime-matrix.json
```

The disposable Darwin source clone, fixture binary, and fixture home were
removed after collection; no scratch checkout or generated binary is part of
the repository.

This closes the rendered cross-platform evidence gap for exact candidate
`eabf8d6` only. A later non-documentation behavior change requires a fresh
three-platform collection. Broad same-UID filesystem TOCTOU remains
`UNVERIFIED`, and release authorization remains `INCONCLUSIVE` without an
operator decision. No v4-completion or release claim follows from this record.
