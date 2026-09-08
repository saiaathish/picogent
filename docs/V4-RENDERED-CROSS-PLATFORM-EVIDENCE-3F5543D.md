# v4 rendered cross-platform evidence at exact candidate `3f5543d`

Status: `PASS` for the rendered cross-platform claim only. This exact-candidate
record satisfies [#507](https://github.com/saiaathish/picogent/issues/507)
under the runtime-boundary parent
[#453](https://github.com/saiaathish/picogent/issues/453). It does not
authorize a release, upgrade the hostile-filesystem residual, or establish
live-provider behavior.

## Candidate and common flow

All three observations used the exact clean behavior candidate
`3f5543d6c88e9484aaf1e01a6eb28f8d7f6982fe` and the same task-owned rendered
recovery fixture:

1. load a fresh fixture in Safe mode;
2. submit the contained recovery task;
3. observe the rendered permission controls and click `Allow`;
4. observe the file change and click `Undo last change`;
5. stop the fixture, start a fresh process, and verify durable history without
   a stale Undo control.

The fixture source was verified clean at the candidate SHA. Each platform
record uses schema `picogent.v4.rendered-platform-evidence.v1`, fixture
`rendered-recovery`, environment `task-owned-disposable`, and `verdict=PASS`.
The platform observations retain digests only; no DOM, URL, credential,
transcript, or screenshot bytes are committed here.

## Platform records

| Platform | Collection | Browser | Observation SHA-256 | Screenshot-set SHA-256 | Platform-record SHA-256 |
| --- | --- | --- | --- | --- | --- |
| `darwin/arm64` | local task-owned collection | `playwright-chromium-headless-151.0.7922.34` | `98dcad3ca8a7cca1a5908e6ca18305f3a21ba75981c9a71d84530f78971b10d7` | `223a27ff98c1b6c087f55c70dfda531fd5767ab44534c5caaea118333df87d39` | `7b56744a984d462efd03b4f3ca56dacc93006e9227f9be609f145039c964d812` |
| `linux/amd64` | [hosted run 34241951536](https://github.com/saiaathish/picogent/actions/runs/34241951536) | `playwright-chromium-headless-134.0.6998.35` | `5290720d9f117b5212c299d2899a872ad7b51e7ae35c4b1b98dfc3d2d3ce124c` | `466c9c0d666523b8250584aa79afc285514f04f3eef81f9f43ac36c3504f8c13` | `23109398f361da4939f32db980813cec6911f4b37f119762f9a407ef8c3969ee` |
| `windows/amd64` | [hosted run 34241951172](https://github.com/saiaathish/picogent/actions/runs/34241951172) | `chrome-headless-151.0.7922.174` | `21f30b36a80677ae4004758af785ad0bd2f2d74f3139d5da69b5a58fc78f019b` | `90c1b715768c3c11b475796e2ef9712c5b62285de9db5b44f7cab2d507ff3d4f` | `a8edf459008d1022457cb804d30b12059f925eb20d3c4145011e274caa896877` |

All three platform runs recorded `source_sha_verified=true` and
`source_tree_modified=false`; the seed and reload fixture processes exited
cleanly. The Darwin collection passed all 18 required checks, and the hosted
Linux and Windows collectors returned `PASS` platform records at the same
candidate SHA.

## Aggregate and exact-candidate matrix

The aggregate validator accepted exactly one valid record for each required
platform:

```text
schema=picogent.v4.rendered-cross-platform-evidence.v1
candidate_sha=3f5543d6c88e9484aaf1e01a6eb28f8d7f6982fe
required_platforms=darwin,linux,windows
verdict=PASS
source_tree_modified=false
aggregate_sha256=be790e046ee209b74531dfbfc7c71a6604f6b6401c214a951f2c54932111f7e5
```

The exact-head runtime matrix was then run from a clean checkout at the same
candidate SHA with both the Darwin local record and the three-platform
aggregate supplied:

```text
candidate_sha=3f5543d6c88e9484aaf1e01a6eb28f8d7f6982fe
behavior_sha=3f5543d6c88e9484aaf1e01a6eb28f8d7f6982fe
behavior_provenance=EXACT_HEAD
head_match=PASS
tree=CLEAN
rendered-platform-local=PASS
rendered-cross-platform=PASS
summary=PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3
runtime_matrix_sha256=2926c24e7c818290ab57c1bfa6caf84a6d11d4a537f66c7e28c3cdc157a84e87
```

The same matrix deliberately retains these independent boundaries:

- `hostile-filesystem-toctou=UNVERIFIED`;
- `live-provider-connectivity=UNVERIFIED`;
- `live-provider-quality=UNVERIFIED`; and
- `release-authorization=INCONCLUSIVE`.

## Operator retention

The source checkout contains documentation only. The local evidence and
downloaded hosted artifacts were retained outside the checkout at:

```text
/private/tmp/picogent-rendered-darwin-3f-collector-output.oIIuHB/
/private/tmp/picogent-rendered-linux-507-artifact.MC0E0Y/
/private/tmp/picogent-rendered-windows-507-artifact.OlfWSr/
/private/tmp/picogent-rendered-cross-platform-3f-aggregate.L5cYfm/aggregate-exact-candidate.json
/private/tmp/picogent-rendered-cross-platform-3f-matrix-local.jGT2s2/exact-candidate-runtime-boundary-matrix.json
```

This closes the rendered cross-platform evidence gap for the exact candidate
`3f5543d` only. A later non-documentation change requires a fresh three-platform
collection. A later documentation-only descendant may retain this record as an
exact-candidate artifact, but must not project its `PASS` onto a different
candidate SHA. No release or v4-completion claim follows from this record.
