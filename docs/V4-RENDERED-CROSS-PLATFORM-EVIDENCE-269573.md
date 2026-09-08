# v4 rendered cross-platform evidence at exact candidate 269573b

Status: `PASS` for the rendered cross-platform claim only. This is an exact
candidate evidence record, not release approval. It records the current-tip
rebind for [#507](https://github.com/saiaathish/picogent/issues/507), after
PR [#564](https://github.com/saiaathish/picogent/pull/564) advanced `main`.

## Candidate and collection

The evidence is bound to candidate SHA
`269573bacfa772a7d14275073ad75397b1aa13a0`, the clean `main` tip at
collection time. Each platform ran the same rendered recovery flow in a
task-owned disposable environment:

1. load a fresh fixture in Safe mode;
2. submit the contained recovery task;
3. observe the rendered permission controls and click `Allow`;
4. observe the file change and click `Undo last change`;
5. stop the fixture, start a fresh process, and verify durable history without
   a stale Undo control.

The fixture binary was built from the exact clean candidate with
`-buildvcs=true -tags rendered_fixture` and reported
`vcs.revision=269573bacfa772a7d14275073ad75397b1aa13a0` and
`vcs.modified=false`. No DOM, URL, credential, transcript, or screenshot is
committed here; the records retain digests only.

| Platform | Collection | Browser | Observation SHA-256 | Screenshot-set SHA-256 | Platform-record SHA-256 |
| --- | --- | --- | --- | --- | --- |
| `darwin/arm64` | local task-owned collection | `playwright-chromium-headless-151.0.7922.34` | `0c444cf527ad57ea66597d993b36c32b1ee16c4cef08f245485b266fc2d73a0a` | `ad46e5a329bbb4952c00afc63a9cc2736eb7006a731971d7c0eda02bf0403819` | `15f304cd73dc9a1fcfc03d5aa14f20114e2048a4e075103dc2eb88b5e5662b0c` |
| `linux/amd64` | [hosted run 34186390698](https://github.com/saiaathish/picogent/actions/runs/34186390698) | `playwright-chromium-headless-134.0.6998.35` | `b7b02299c1f6050feff8f4df0a88155b821234a016acc55a127462a66f70ad69` | `688e07d4f4610b628d7032347a6d3b714ff26f10972ac50cb37410ac5d21f02b` | `25470bfa17c5bd9cbeab4c6739c740548080ce87cc0f32dbd8df5ba7eafcec06` |
| `windows/amd64` | [hosted run 34186390642](https://github.com/saiaathish/picogent/actions/runs/34186390642) | `chrome-headless-151.0.7922.174` | `8775ec803fe3793e693d84078b91d200695e79307a8e2663cb5c71c68c591e14` | `0a118bed9c3d5bca0397c8bca800a9e5072e63fe00e5b4de94d47ba9605a9034` | `c0ae8e880f4b0173245bc756a90b147a1dfc13df0ebc5b9c6d731af2250645d6` |

All three platform records use schema
`picogent.v4.rendered-platform-evidence.v1`, fixture `rendered-recovery`,
environment `task-owned-disposable`, `verdict=PASS`, and
`source_tree_modified=false`. The local Darwin collection and both hosted
jobs verified fixture exit and exact source provenance.

## Aggregate and matrix validation

The aggregate was packaged from one validated record per platform in a clean
checkout at the same candidate SHA:

```text
schema=picogent.v4.rendered-cross-platform-evidence.v1
candidate_sha=269573bacfa772a7d14275073ad75397b1aa13a0
required_platforms=darwin,linux,windows
verdict=PASS
source_tree_modified=false
aggregate_sha256=b05fcf40f7236bba4ff7fcae30a11c3015f38246d61d9a5f5cb4e087f9e7951c
```

The exact-head runtime matrix was then run with that aggregate and retained
outside the checkout:

```text
candidate_sha=269573bacfa772a7d14275073ad75397b1aa13a0
behavior_provenance=EXACT_HEAD
head_match=PASS
tree=CLEAN
rendered-cross-platform=PASS
summary=PASS 7 / INCONCLUSIVE 1 / UNVERIFIED 4
runtime_matrix_sha256=07b1f929149af91212574ef88ca16c800ef21a8a264f63cd74182530d9d32f9b
```

The same matrix deliberately retains these boundaries:

- `hostile-filesystem-toctou=UNVERIFIED`;
- `live-provider-connectivity=UNVERIFIED`;
- `live-provider-quality=UNVERIFIED`; and
- `release-authorization=INCONCLUSIVE`.

Those rows are independent claims. This rendered aggregate does not authorize
a release or establish arbitrary same-UID filesystem race safety.

## Operator retention

The source checkout contains documentation only. The local evidence and
downloaded hosted artifacts were retained outside the checkout at:

```text
/private/tmp/picogent-rendered-darwin-269573-evidence-clone/
/private/tmp/picogent-rendered-linux-269573/
/private/tmp/picogent-rendered-windows-269573/
/private/tmp/picogent-rendered-cross-platform-269573/aggregate-exact-candidate.json
/private/tmp/picogent-rendered-cross-platform-269573/exact-candidate-runtime-boundary-matrix.json
```

The aggregate closes the rendered cross-platform evidence gap for this exact
candidate only. Later non-documentation changes require a fresh three-platform
collection or an explicit `UNVERIFIED` result.
