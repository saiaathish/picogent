# v4 rendered cross-platform evidence at exact candidate `1ce2059`

Status: `PASS` for the rendered cross-platform claim only. This exact-candidate
record belongs to [#507](https://github.com/saiaathish/picogent/issues/507) under
the runtime-boundary parent [#453](https://github.com/saiaathish/picogent/issues/453).
It does not authorize a release or upgrade the hostile-filesystem residual.

## Candidate and common flow

All three observations used the exact clean candidate SHA
`1ce2059fc352b5d32b5da3adbaeb1ecb48834db7` and the same task-owned rendered
recovery fixture:

1. load a fresh fixture in Safe mode;
2. submit the contained recovery task;
3. observe the permission controls and click `Allow`;
4. observe the file change and click `Undo last change`;
5. stop the fixture, start a fresh process, and verify durable history without a
   stale recovery control.

Each platform record reports `fixture=rendered-recovery`,
`environment=task-owned-disposable`, `verdict=PASS`, and
`source_tree_modified=false`. The hosted jobs verified clean exact-SHA source
and fixture provenance. The Darwin record was collected locally from a clean
clone with the repository's Playwright Chromium collector; its browser identity
is recorded below and is not a BrowserOS-specific claim.

## Platform records

| Platform | Collection | Browser | Observation SHA-256 | Screenshot-set SHA-256 | Platform-record SHA-256 |
| --- | --- | --- | --- | --- | --- |
| `darwin/arm64` | local task-owned collection | `playwright-chromium-headless-151.0.7922.34` | `d17ed3651f764c72e6823efbff222626a7b24ba132c2e1359c1dd5da3323d7d9` | `e9046c3d6291bf6172cfdddea01ad832ea5467299d2df02a9b59b28df90c2e71` | `8d84f3f16b95360e79272825b820f3c26266016cd8bc98e58b93554e1b378e7c` |
| `linux/amd64` | [hosted run 34222585513](https://github.com/saiaathish/picogent/actions/runs/34222585513) | `playwright-chromium-headless-134.0.6998.35` | `53dab136eba607ed0550f93e76f3248cba14e0ed13f938b7ef2937b3fdb73225` | `c116fb0f02c13827ea0397141e60653aa12565981434d316cbf9a909d3146617` | `c7d85be00f046dcb8fffe946001f86dd21f0ea3997c1694078fa17c1ad2a389b` |
| `windows/amd64` | [hosted run 34222591261](https://github.com/saiaathish/picogent/actions/runs/34222591261) | `chrome-headless-151.0.7922.174` | `95798c6c35ed6023e5a67855f91fe11bca1aa9bca2ca20d2f671263996acaad5` | `1b2c675bf221dfb3c952e29f2df42302f0ddf58e9c6f9362d1815ef8fe1cdea1` | `5b0f77447a5f89a478ca9482be75c30007d8a49343ebcbe032c74fb83d8cf3de` |

The local Darwin collector recorded `fixture_exit_verified=true` and
`observed_at=2026-09-08T11:47:18Z`. The Linux collector recorded
`observed_at=2026-09-08T11:48:49Z`; the Windows record recorded
`observed_at=2026-09-08T11:49:06.782Z`.

## Aggregate and exact-head matrix

The aggregate validator accepted exactly one valid record for each required
platform:

```text
schema=picogent.v4.rendered-cross-platform-evidence.v1
candidate_sha=1ce2059fc352b5d32b5da3adbaeb1ecb48834db7
required_platforms=darwin,linux,windows
verdict=PASS
source_tree_modified=false
aggregate_sha256=046cc982c120f21b23c07b2ec5f81f5ff0473419bd39d478449913e375aa68ea
```

The retained exact-head runtime matrix, with the live-provider artifacts and
the rendered aggregate supplied, recorded:

```text
candidate_sha=1ce2059fc352b5d32b5da3adbaeb1ecb48834db7
head_match=PASS  tree=CLEAN  behavior_provenance=EXACT_HEAD
summary=PASS 10 / INCONCLUSIVE 1 / UNVERIFIED 1
rendered-platform-local=PASS
rendered-cross-platform=PASS
live-provider-connectivity=PASS
live-provider-quality=PASS
hostile-filesystem-toctou=UNVERIFIED
release-authorization=INCONCLUSIVE
runtime_matrix_sha256=c0e56a26407c19828a9e1a76e6d8a8e229cdaf6ffc1c7b0b7056ba52ea7ed6c7
```

The digest-only artifacts and downloaded hosted records remain outside the
checkout under `/private/tmp/picogent-evidence-1ce2059/`:

```text
darwin-rendered-clone/darwin-rendered-platform-evidence.json
linux/picogent-linux-rendered-evidence/linux-rendered-platform-evidence.json
windows/windows-rendered-platform.json
rendered-cross-platform-evidence.json
runtime-boundary-matrix-full.json
```

No DOM, URL, credential, transcript, or screenshot bytes are committed here.

## Explicit limits

- This aggregate proves the named rendered recovery fixture on Darwin, Linux,
  and Windows at this exact candidate only.
- `hostile-filesystem-toctou` remains `UNVERIFIED`; parent-swap confinement and
  final-path observations are narrower claims.
- Live-provider connectivity and quality are separate bounded observations in
  [V4-LIVE-PROVIDER-EVIDENCE-1CE2059.md](V4-LIVE-PROVIDER-EVIDENCE-1CE2059.md).
- `release-authorization` remains `INCONCLUSIVE` without the required human
  operator decision.
- A later non-documentation change requires a fresh three-platform collection.
  Docs-only descendants must follow the repository's exact-SHA continuity
  rules and must not project this aggregate onto a different candidate SHA.
