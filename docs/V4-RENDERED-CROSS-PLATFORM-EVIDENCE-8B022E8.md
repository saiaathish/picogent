# v4 rendered cross-platform evidence at exact candidate `8b022e8`

Status: `PASS` for the rendered local-platform and cross-platform claims only.
This is an exact-candidate evidence refresh under runtime-boundary parent
[#453](https://github.com/saiaathishkarthik/picogent/issues/453). It does not
authorize a release, upgrade the hostile-filesystem residual, establish
live-provider behavior, or claim v4 completion.

## Candidate and common flow

All three observations used the exact clean behavior candidate
`8b022e82be4b2e8ade0a502c5f373eaf6dc61160` and the same task-owned rendered
recovery fixture:

1. load a fresh fixture in Safe mode;
2. submit the contained recovery task;
3. observe the rendered permission controls and click `Allow`;
4. observe the file change and click `Undo last change`;
5. stop the fixture, start a fresh process, and verify durable history without
   a stale Undo control.

Each platform record uses schema
`picogent.v4.rendered-platform-evidence.v1`, fixture `rendered-recovery`,
environment `task-owned-disposable`, and `verdict=PASS`. The platform
observations retain digests only; no DOM, URL, credential, transcript, or
screenshot bytes are committed here.

## Platform records

| Platform | Collection | Browser | Observation SHA-256 | Screenshot-set SHA-256 | Platform-record SHA-256 |
| --- | --- | --- | --- | --- | --- |
| `darwin/arm64` | local task-owned collection | `playwright-chromium-headless-151.0.7922.34` | `97097a67e8e8bdb0f28b87019ddccd4cddd088b153b0df78e9715207b9f8c0e8` | `e461c4ff6869e13bb1e886c3c73155a1c96e568eb3836e92830d8d15f4fcba3d` | `2226426e8845d228c8a276e8dfb5799676e23994fc71fae1113b5117a62dc99b` |
| `linux/amd64` | [hosted run 34463643641](https://github.com/saiaathishkarthik/picogent/actions/runs/34463643641) | `playwright-chromium-headless-134.0.6998.35` | `dd6e473cbff51ccd5aaad9b09c16ac1d786f135f9c936511c7a65495a9dc2803` | `ab11acc557dc2de22939843a563963ad1af00089f1d03fe42cd4a2a6261a5ebc` | `0b8b43f1883aa58df1d07291031a44ec7050069c579693b01ab3d39aa4f8047e` |
| `windows/amd64` | [hosted run 34463643642](https://github.com/saiaathishkarthik/picogent/actions/runs/34463643642) | `chrome-headless-152.0.7977.83` | `50a397e107907f4e0aae8873803a895f67fd5c95ab162f4a5966ee0ca619e10d` | `c3829d430052ba8cbcd16c7aa5eda5098c9f1316cb349fed91b0835ab617eb93` | `26999a3c7c0ce52125662b633094eaa623d40d51714e2bf61f05fb8b808f3bf5` |

All three records report `source_sha_verified=true` and
`source_tree_modified=false`; both fixture processes exited cleanly. The
Darwin collection passed all 18 required checks, and the hosted Linux and
Windows collectors returned `PASS` records at the same candidate SHA.

## Aggregate and exact-candidate matrix

The aggregate validator accepted exactly one valid record for each required
platform:

```text
schema=picogent.v4.rendered-cross-platform-evidence.v1
candidate_sha=8b022e82be4b2e8ade0a502c5f373eaf6dc61160
required_platforms=darwin,linux,windows
verdict=PASS
source_tree_modified=false
aggregate_sha256=47f62799caa8dd11b655adff287a5ab1b29c4835b8e777832eb7fae2079bca04
```

The exact-head runtime matrix was generated from the clean checkout at the
same candidate SHA with the Darwin local record and three-platform aggregate
supplied:

```text
candidate_sha=8b022e82be4b2e8ade0a502c5f373eaf6dc61160
behavior_sha=8b022e82be4b2e8ade0a502c5f373eaf6dc61160
behavior_provenance=EXACT_HEAD
head_match=PASS
tree=CLEAN
rendered-platform-local=PASS
rendered-cross-platform=PASS
summary=PASS 8 / INCONCLUSIVE 1 / UNVERIFIED 3
runtime_matrix_sha256=b1a735b0a377d2b519e6306fb11308ef10498a5d612680e4a858d4c4ad772ae2
```

The matrix intentionally retains these independent boundaries:

- `hostile-filesystem-toctou=UNVERIFIED`;
- `live-provider-connectivity=UNVERIFIED`;
- `live-provider-quality=UNVERIFIED`; and
- `release-authorization=INCONCLUSIVE`.

## Separate live-provider continuity

The retained live-provider artifacts are bound to behavior SHA `b7b5c5c`, not
to the rendered candidate. Their current-main continuity refresh is recorded
separately in
[V4-LIVE-PROVIDER-CONTINUITY-8B022E8.md](V4-LIVE-PROVIDER-CONTINUITY-8B022E8.md).
The runtime-boundary contract binds supplied external artifacts to one
behavior SHA, so these two matrices must not be unioned into a synthetic
release-authorizing result.

## Operator retention

The source checkout contains documentation only. The platform evidence and
aggregate artifacts were retained outside the checkout at:

```text
/private/tmp/picogent-rendered-current-8b022e8/darwin/evidence/
/private/tmp/picogent-rendered-current-8b022e8/linux/rendered-linux-owned-browser-8b022e82be4b2e8ade0a502c5f373eaf6dc61160/picogent-linux-rendered-evidence/
/private/tmp/picogent-rendered-current-8b022e8/windows/rendered-windows-evidence-8b022e82be4b2e8ade0a502c5f373eaf6dc61160/
/private/tmp/picogent-rendered-current-8b022e8/aggregate/rendered-cross-platform-evidence.json
/private/tmp/picogent-rendered-current-8b022e8/aggregate/runtime-boundary-matrix-rendered.json
```

This closes the rendered cross-platform evidence gap for the exact candidate
`8b022e8` only. A later non-documentation change requires a fresh
three-platform collection. No release or v4-completion claim follows from
this record.
